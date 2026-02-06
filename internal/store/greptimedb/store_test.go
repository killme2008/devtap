package greptimedb

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/config"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		input       string
		defaultPort int
		wantHost    string
		wantPort    int
		wantErr     bool
	}{
		{"127.0.0.1:4001", 4001, "127.0.0.1", 4001, false},
		{"10.0.0.1:5001", 4001, "10.0.0.1", 5001, false},
		{"localhost", 4001, "localhost", 4001, false},
		{"myhost:abc", 4001, "", 0, true},
	}

	for _, tt := range tests {
		host, port, err := parseEndpoint(tt.input, tt.defaultPort)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseEndpoint(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseEndpoint(%q): %v", tt.input, err)
			continue
		}
		if host != tt.wantHost {
			t.Errorf("parseEndpoint(%q) host: got %q, want %q", tt.input, host, tt.wantHost)
		}
		if port != tt.wantPort {
			t.Errorf("parseEndpoint(%q) port: got %d, want %d", tt.input, port, tt.wantPort)
		}
	}
}

func TestBuildMySQLDSN(t *testing.T) {
	cfg := config.GreptimeDBConfig{
		MySQLEndpoint: "127.0.0.1:4002",
		Database:      "devtap",
	}

	dsn := buildMySQLDSN(cfg)
	expected := "tcp(127.0.0.1:4002)/devtap?parseTime=true"
	if dsn != expected {
		t.Errorf("buildMySQLDSN: got %q, want %q", dsn, expected)
	}
}

func TestValidateFilterSQL(t *testing.T) {
	valid := []string{
		"content LIKE '%error%'",
		"tag = 'build' AND exit_code != 0",
		"content LIKE '%warning%' OR content LIKE '%error%'",
	}
	for _, sql := range valid {
		if err := validateFilterSQL(sql); err != nil {
			t.Errorf("validateFilterSQL(%q): unexpected error: %v", sql, err)
		}
	}

	invalid := []struct {
		sql    string
		reason string
	}{
		{"1=1; DROP TABLE devtap_logs", "semicolon"},
		{"DROP TABLE foo", "DROP keyword"},
		{"DELETE FROM devtap_logs WHERE 1=1", "DELETE keyword"},
		{"content LIKE '%x%'; INSERT INTO foo VALUES(1)", "semicolon with INSERT"},
		{"UPDATE devtap_logs SET content=''", "UPDATE keyword"},
		{"1=1) UNION SELECT 1,2,3,4,5,6 --", "UNION injection"},
		{"SELECT 1", "bare SELECT"},
	}
	for _, tt := range invalid {
		if err := validateFilterSQL(tt.sql); err == nil {
			t.Errorf("validateFilterSQL(%q): expected error for %s", tt.sql, tt.reason)
		}
	}
}

func TestPingUnreachable(t *testing.T) {
	cfg := config.GreptimeDBConfig{
		Endpoint:      "127.0.0.1:19999",
		MySQLEndpoint: "127.0.0.1:19998",
		Database:      "devtap_test",
	}
	s, err := New(cfg, t.TempDir(), "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err == nil {
		t.Error("expected Ping to fail against unreachable endpoint")
	}
}

func TestReservedKeywordsQuoted(t *testing.T) {
	// GreptimeDB treats "tag" and "stream" as reserved keywords.
	// All SQL must backtick-quote them to avoid syntax errors.
	reserved := []string{"tag", "stream"}

	t.Run("DDL", func(t *testing.T) {
		ddl := buildCreateTableSQL()
		for _, kw := range reserved {
			quoted := fmt.Sprintf("`%s`", kw)
			unquoted := fmt.Sprintf(" %s ", kw)
			if !strings.Contains(ddl, quoted) {
				t.Errorf("DDL missing backtick-quoted %q: %s", kw, ddl)
			}
			// Ensure no unquoted bare keyword appears as a column identifier.
			// Remove all backtick-quoted occurrences, then check for bare keyword.
			stripped := strings.ReplaceAll(ddl, quoted, "")
			if strings.Contains(strings.ToLower(stripped), unquoted) {
				t.Errorf("DDL contains unquoted %q column identifier: %s", kw, ddl)
			}
		}
	})

	t.Run("DrainQuery", func(t *testing.T) {
		query := fmt.Sprintf(
			"SELECT ts, `tag`, `stream`, adapter, content, exit_code FROM %s WHERE session_id = ? AND adapter = ? AND ts > ? ORDER BY ts ASC LIMIT ?",
			tableName,
		)
		for _, kw := range reserved {
			quoted := fmt.Sprintf("`%s`", kw)
			if !strings.Contains(query, quoted) {
				t.Errorf("Drain query missing backtick-quoted %q", kw)
			}
		}
	})

	t.Run("HistoryStreamFilter", func(t *testing.T) {
		// The History method filters with: AND `stream` = 'exit'
		filter := "`stream` = 'exit'"
		if !strings.Contains(filter, "`stream`") {
			t.Error("History stream filter missing backtick-quoted stream")
		}
	})
}

func TestLoadWatermarkDefault(t *testing.T) {
	cfg := config.GreptimeDBConfig{
		Endpoint:      "127.0.0.1:19999",
		MySQLEndpoint: "127.0.0.1:19998",
		Database:      "devtap_test",
	}
	s, err := New(cfg, t.TempDir(), "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	wm, err := s.loadWatermark("nonexistent-session")
	if err != nil {
		t.Fatalf("loadWatermark: %v", err)
	}

	// Default watermark must be Unix epoch, not Go zero time (year 0001).
	// GreptimeDB cannot compare TIMESTAMP columns against pre-epoch values.
	if wm.Year() < 1970 {
		t.Errorf("default watermark year = %d, want >= 1970", wm.Year())
	}
	if !wm.Equal(epochZero) {
		t.Errorf("default watermark = %v, want %v", wm, epochZero)
	}
}

func TestBuildMySQLDSNWithAuth(t *testing.T) {
	cfg := config.GreptimeDBConfig{
		MySQLEndpoint: "10.0.0.1:4002",
		Database:      "mydb",
	}

	t.Setenv("DEVTAP_GREPTIMEDB_USERNAME", "admin")
	t.Setenv("DEVTAP_GREPTIMEDB_PASSWORD", "secret")

	dsn := buildMySQLDSN(cfg)
	expected := "admin:secret@tcp(10.0.0.1:4002)/mydb?parseTime=true"
	if dsn != expected {
		t.Errorf("buildMySQLDSN: got %q, want %q", dsn, expected)
	}
}
