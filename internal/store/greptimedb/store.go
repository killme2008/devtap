package greptimedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	greptime "github.com/GreptimeTeam/greptimedb-ingester-go"
	"github.com/GreptimeTeam/greptimedb-ingester-go/table"
	"github.com/GreptimeTeam/greptimedb-ingester-go/table/types"
	_ "github.com/go-sql-driver/mysql"

	"github.com/killme2008/devtap/internal/config"
	"github.com/killme2008/devtap/internal/store"
)

const (
	tableName     = "devtap_logs"
	watermarkFile = "watermark"
)

// Store implements store.Store using GreptimeDB as the backend.
type Store struct {
	client  *greptime.Client
	db      *sql.DB
	cfg     config.GreptimeDBConfig
	baseDir string // for watermark files (~/.devtap)
	adapter string // adapter name
	ensured bool   // whether table has been created
}

// New creates a new GreptimeDB-backed store.
func New(cfg config.GreptimeDBConfig, baseDir string, adapter string) (*Store, error) {
	// Parse host and port from endpoint
	host, port, err := parseEndpoint(cfg.Endpoint, 4001)
	if err != nil {
		return nil, fmt.Errorf("parse gRPC endpoint: %w", err)
	}

	// Build ingester config
	greptimeCfg := greptime.NewConfig(host).
		WithPort(port).
		WithDatabase(cfg.Database)

	username := cfg.Username()
	password := cfg.Password()
	if username != "" {
		greptimeCfg = greptimeCfg.WithAuth(username, password)
	}

	client, err := greptime.NewClient(greptimeCfg)
	if err != nil {
		return nil, fmt.Errorf("create greptime client: %w", err)
	}

	// Build MySQL DSN for queries
	dsn := buildMySQLDSN(cfg)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("open mysql connection: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	s := &Store{
		client:  client,
		db:      db,
		cfg:     cfg,
		baseDir: baseDir,
		adapter: adapter,
	}

	return s, nil
}

// Write appends a log message to GreptimeDB for all registered adapters.
// This fans out the message so every adapter can independently drain its own copy.
func (s *Store) Write(sessionID string, msg store.LogMessage) error {
	if err := s.ensureTable(); err != nil {
		return fmt.Errorf("ensure table: %w", err)
	}

	adapters := store.DiscoverAdapters(s.baseDir, sessionID, s.adapter)

	tbl, err := table.New(tableName)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	if err := tbl.AddTagColumn("session_id", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddTagColumn("tag", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddTagColumn("stream", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddTagColumn("adapter", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddFieldColumn("content", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddFieldColumn("exit_code", types.INT32); err != nil {
		return err
	}
	if err := tbl.AddFieldColumn("host", types.STRING); err != nil {
		return err
	}
	if err := tbl.AddTimestampColumn("ts", types.TIMESTAMP_MICROSECOND); err != nil {
		return err
	}

	content := strings.Join(msg.Lines, "\n")
	exitCode := int32(0)
	if msg.ExitCode != nil {
		exitCode = int32(*msg.ExitCode)
	}

	ts := msg.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	for _, adapter := range adapters {
		if err := tbl.AddRow(sessionID, msg.Tag, msg.Stream, adapter, content, exitCode, msg.Host, ts); err != nil {
			return fmt.Errorf("add row: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = s.client.Write(ctx, tbl)
	if err != nil {
		return fmt.Errorf("write to greptimedb: %w", err)
	}

	return nil
}

// Drain reads pending messages since the last watermark for a session.
//
// Known limitation: the watermark is set to the max timestamp of returned rows.
// If more than maxLines rows share the exact same microsecond timestamp, rows
// beyond the limit are skipped. This is practically unreachable given TIMESTAMP(6)
// microsecond precision and the default 10000-row limit.
func (s *Store) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	query := fmt.Sprintf(
		"SELECT ts, `tag`, `stream`, adapter, content, exit_code, COALESCE(host, '') AS host FROM %s WHERE session_id = ? AND adapter = ? AND ts > ? ORDER BY ts ASC LIMIT ?",
		tableName,
	)
	return s.drainWithQuery(sessionID, maxLines, query)
}

// DrainSQL executes a custom SQL filter query for drain.
func (s *Store) DrainSQL(sessionID string, filterSQL string, maxLines int) ([]store.LogMessage, error) {
	if err := validateFilterSQL(filterSQL); err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		"SELECT ts, `tag`, `stream`, adapter, content, exit_code, COALESCE(host, '') AS host FROM %s WHERE session_id = ? AND adapter = ? AND ts > ? AND (%s) ORDER BY ts ASC LIMIT ?",
		tableName, filterSQL,
	)
	return s.drainWithQuery(sessionID, maxLines, query)
}

// validateFilterSQL is a best-effort blocklist for --filter-sql expressions.
// This is NOT a security boundary — the user already has local CLI and DB access.
// It prevents accidental misuse, not adversarial injection.
func validateFilterSQL(sql string) error {
	upper := strings.ToUpper(sql)
	for _, keyword := range []string{
		"DROP ", "DELETE ", "INSERT ", "UPDATE ", "ALTER ", "CREATE ",
		"TRUNCATE ", "GRANT ", "REVOKE ", "UNION ", "SELECT ",
	} {
		if strings.Contains(upper, keyword) {
			return fmt.Errorf("filter-sql must be a WHERE clause expression, not a statement (found %q)", strings.TrimSpace(keyword))
		}
	}
	if strings.Contains(sql, ";") {
		return fmt.Errorf("filter-sql must not contain semicolons")
	}
	return nil
}

// drainWithQuery is the shared implementation for Drain and DrainSQL.
func (s *Store) drainWithQuery(sessionID string, maxLines int, query string) ([]store.LogMessage, error) {
	if err := s.ensureTable(); err != nil {
		return nil, fmt.Errorf("ensure table: %w", err)
	}

	// Ensure own adapter dir exists so writers can discover and fan-out to us.
	if err := store.EnsureAdapterDir(s.baseDir, sessionID, s.adapter); err != nil {
		return nil, fmt.Errorf("ensure adapter dir: %w", err)
	}

	if maxLines <= 0 {
		maxLines = 10000
	}

	watermark, err := s.loadWatermark(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load watermark: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query, sessionID, s.adapter, watermark.Format("2006-01-02 15:04:05.999999"), maxLines)
	if err != nil {
		return nil, fmt.Errorf("query greptimedb: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []store.LogMessage
	var maxTS time.Time

	for rows.Next() {
		var (
			ts          time.Time
			tag         string
			stream      string
			adapterName string
			content     string
			exitCode    int32
			host        string
		)
		if err := rows.Scan(&ts, &tag, &stream, &adapterName, &content, &exitCode, &host); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		msg := store.LogMessage{
			Timestamp: ts,
			Tag:       tag,
			Stream:    stream,
			Adapter:   adapterName,
			Host:      host,
		}
		if content != "" {
			msg.Lines = strings.Split(content, "\n")
		}
		if exitCode != 0 || stream == "exit" {
			ec := int(exitCode)
			msg.ExitCode = &ec
		}

		messages = append(messages, msg)
		if ts.After(maxTS) {
			maxTS = ts
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	if !maxTS.IsZero() {
		if err := s.saveWatermark(sessionID, maxTS); err != nil {
			return nil, fmt.Errorf("save watermark: %w", err)
		}
	}

	return messages, nil
}

// Status returns pending message counts per session (since last watermark).
func (s *Store) Status() (map[string]int, error) {
	if err := s.ensureTable(); err != nil {
		return nil, fmt.Errorf("ensure table: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := fmt.Sprintf("SELECT session_id, COUNT(*) as cnt FROM %s WHERE adapter = ? GROUP BY session_id", tableName)
	rows, err := s.db.QueryContext(ctx, query, s.adapter)
	if err != nil {
		return nil, fmt.Errorf("query status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			continue
		}
		// Subtract already-drained messages by checking watermark
		watermark, err := s.loadWatermark(sessionID)
		if err != nil {
			result[sessionID] = count
			continue
		}

		// Count only messages after watermark
		var pending int
		countQuery := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE session_id = ? AND adapter = ? AND ts > ?",
			tableName,
		)
		err = s.db.QueryRowContext(ctx, countQuery, sessionID, s.adapter, watermark.Format("2006-01-02 15:04:05.999999")).Scan(&pending)
		if err != nil {
			result[sessionID] = count
			continue
		}
		if pending > 0 {
			result[sessionID] = pending
		}
	}

	return result, rows.Err()
}

// StatusExtended returns richer statistics per session (GreptimeDB-specific).
func (s *Store) StatusExtended() ([]SessionStats, error) {
	if err := s.ensureTable(); err != nil {
		return nil, fmt.Errorf("ensure table: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT session_id, `+"`tag`"+`, adapter, COUNT(*) as total,
		       SUM(CASE WHEN exit_code != 0 THEN 1 ELSE 0 END) as errors,
		       MAX(ts) as last_seen
		FROM %s
		GROUP BY session_id, `+"`tag`"+`, adapter
		ORDER BY last_seen DESC
	`, tableName)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query extended status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []SessionStats
	for rows.Next() {
		var st SessionStats
		if err := rows.Scan(&st.SessionID, &st.Tag, &st.Adapter, &st.Total, &st.Errors, &st.LastSeen); err != nil {
			continue
		}
		stats = append(stats, st)
	}

	return stats, rows.Err()
}

// SessionStats holds extended session statistics.
type SessionStats struct {
	SessionID string
	Tag       string
	Adapter   string
	Total     int
	Errors    int
	LastSeen  time.Time
}

// HistoryEntry represents a single historical build event.
type HistoryEntry struct {
	Timestamp time.Time
	Tag       string
	Adapter   string
	ExitCode  int
	Summary   string // first line of content
	Host      string
}

// History queries historical build events for a session.
func (s *Store) History(sessionID, tag string, since time.Duration, limit int) ([]HistoryEntry, error) {
	if err := s.ensureTable(); err != nil {
		return nil, fmt.Errorf("ensure table: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sinceTime := time.Now().UTC().Add(-since).Format("2006-01-02 15:04:05.999999")

	var queryBuilder strings.Builder
	queryBuilder.WriteString(fmt.Sprintf(
		"SELECT ts, `tag`, adapter, exit_code, content, COALESCE(host, '') AS host FROM %s WHERE session_id = ? AND adapter = ? AND ts > ?",
		tableName,
	))

	queryArgs := []any{sessionID, s.adapter, sinceTime}

	if tag != "" {
		queryBuilder.WriteString(" AND `tag` = ?")
		queryArgs = append(queryArgs, tag)
	}

	// Focus on exit events for a cleaner history view
	queryBuilder.WriteString(" AND `stream` = 'exit'")
	queryBuilder.WriteString(" ORDER BY ts DESC")
	queryBuilder.WriteString(fmt.Sprintf(" LIMIT %d", limit))

	rows, err := s.db.QueryContext(ctx, queryBuilder.String(), queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}

	entries := scanHistoryEntries(rows)
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history rows: %w", err)
	}

	// If exit-stream query returned no results, retry without the stream filter.
	if len(entries) == 0 {
		queryBuilder.Reset()
		queryBuilder.WriteString(fmt.Sprintf(
			"SELECT ts, `tag`, adapter, exit_code, content, COALESCE(host, '') AS host FROM %s WHERE session_id = ? AND adapter = ? AND ts > ?",
			tableName,
		))
		queryArgs = []any{sessionID, s.adapter, sinceTime}
		if tag != "" {
			queryBuilder.WriteString(" AND `tag` = ?")
			queryArgs = append(queryArgs, tag)
		}
		queryBuilder.WriteString(" ORDER BY ts DESC")
		queryBuilder.WriteString(fmt.Sprintf(" LIMIT %d", limit))

		rows, err = s.db.QueryContext(ctx, queryBuilder.String(), queryArgs...)
		if err != nil {
			return nil, fmt.Errorf("query history fallback: %w", err)
		}
		entries = scanHistoryEntries(rows)
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate history rows: %w", err)
		}
	}

	return entries, nil
}

func scanHistoryEntries(rows *sql.Rows) []HistoryEntry {
	var entries []HistoryEntry
	for rows.Next() {
		var (
			ts          time.Time
			entryTag    string
			adapterName string
			exitCode    int32
			content     string
			host        string
		)
		if err := rows.Scan(&ts, &entryTag, &adapterName, &exitCode, &content, &host); err != nil {
			continue
		}

		summary := content
		if idx := strings.Index(content, "\n"); idx >= 0 {
			summary = content[:idx]
		}

		entries = append(entries, HistoryEntry{
			Timestamp: ts,
			Tag:       entryTag,
			Adapter:   adapterName,
			ExitCode:  int(exitCode),
			Summary:   summary,
			Host:      host,
		})
	}
	return entries
}

// Ping checks connectivity to GreptimeDB using the always-present "public" database.
func (s *Store) Ping(ctx context.Context) error {
	dsn := buildMySQLDSNForDB(s.cfg, "public")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return db.PingContext(ctx)
}

// Close releases client resources.
func (s *Store) Close() error {
	_ = s.client.Close()
	return s.db.Close()
}

// ensureTable creates the database and table if they don't exist.
func (s *Store) ensureTable() error {
	if s.ensured {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create database if it doesn't exist.
	// Use a temporary connection without a database name since the
	// main connection pool targets the database which may not exist yet.
	if err := s.ensureDatabase(ctx); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}

	createSQL := buildCreateTableSQL()

	_, err := s.db.ExecContext(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// Best-effort migration: add host column to existing tables.
	// Silently ignore error — column may already exist (new table), or
	// the ALTER may fail for other reasons (not critical).
	_, _ = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN host STRING", tableName))

	s.ensured = true
	return nil
}

// ensureDatabase creates the configured database if it doesn't exist.
func (s *Store) ensureDatabase(ctx context.Context) error {
	dsn := buildMySQLDSNNoDB(s.cfg)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.cfg.Database))
	return err
}

// watermark management

func (s *Store) watermarkPath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID, s.adapter, watermarkFile)
}

// epochZero is the minimum safe watermark timestamp (Unix epoch).
// Using time.Time{} (year 0001) would produce a pre-epoch string that
// GreptimeDB cannot compare correctly with TIMESTAMP columns.
var epochZero = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func (s *Store) loadWatermark(sessionID string) (time.Time, error) {
	data, err := os.ReadFile(s.watermarkPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return epochZero, nil
		}
		return epochZero, err
	}

	var ts time.Time
	if err := json.Unmarshal(data, &ts); err != nil {
		return epochZero, nil // corrupted, reset
	}
	return ts, nil
}

func (s *Store) saveWatermark(sessionID string, ts time.Time) error {
	dir := filepath.Dir(s.watermarkPath(sessionID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	return os.WriteFile(s.watermarkPath(sessionID), data, 0o644)
}

// helpers

// buildCreateTableSQL returns the DDL for the devtap_logs table.
// Reserved keywords (tag, stream) are backtick-quoted.
func buildCreateTableSQL() string {
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s ("+
		" ts TIMESTAMP(6) TIME INDEX,"+
		" session_id STRING,"+
		" `tag` STRING,"+
		" `stream` STRING,"+
		" adapter STRING,"+
		" content STRING,"+
		" exit_code INT,"+
		" host STRING,"+
		" PRIMARY KEY (session_id, `tag`, `stream`, adapter)"+
		") WITH ('append_mode'='true', 'ttl'='7d')", tableName)
}

func parseEndpoint(endpoint string, defaultPort int) (string, int, error) {
	parts := strings.SplitN(endpoint, ":", 2)
	host := parts[0]
	port := defaultPort
	if len(parts) == 2 {
		if _, err := fmt.Sscanf(parts[1], "%d", &port); err != nil {
			return "", 0, fmt.Errorf("invalid port in endpoint %q: %w", endpoint, err)
		}
	}
	return host, port, nil
}

func buildMySQLDSN(cfg config.GreptimeDBConfig) string {
	return buildMySQLDSNForDB(cfg, cfg.Database)
}

func buildMySQLDSNNoDB(cfg config.GreptimeDBConfig) string {
	return buildMySQLDSNForDB(cfg, "")
}

func buildMySQLDSNForDB(cfg config.GreptimeDBConfig, database string) string {
	host, port, err := parseEndpoint(cfg.MySQLEndpoint, 4002)
	if err != nil {
		host = "127.0.0.1"
		port = 4002
	}

	username := cfg.Username()
	password := cfg.Password()

	// Build DSN: [user[:password]@][tcp(host:port)]/dbname[?params]
	var dsn strings.Builder
	if username != "" {
		dsn.WriteString(username)
		if password != "" {
			dsn.WriteString(":")
			dsn.WriteString(password)
		}
		dsn.WriteString("@")
	}
	dsn.WriteString(fmt.Sprintf("tcp(%s:%d)/%s?parseTime=true", host, port, database))

	return dsn.String()
}
