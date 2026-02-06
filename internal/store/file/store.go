package file

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/killme2008/devtap/internal/store"
)

const (
	pendingFile    = "pending.jsonl"
	drainingFile   = "pending.jsonl.draining"
	defaultMaxLen  = 10000
	scannerInitBuf = 64 * 1024   // 64 KB initial scanner buffer
	scannerMaxBuf  = 1024 * 1024 // 1 MB max scanner buffer
)

// Store implements store.Store using JSONL files with atomic rename for concurrency safety.
type Store struct {
	baseDir string // ~/.devtap
	adapter string // adapter name, used as subdirectory under session
}

// New creates a new file-based store rooted at baseDir.
func New(baseDir string, adapter string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	return &Store{baseDir: baseDir, adapter: adapter}, nil
}

func (s *Store) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID, s.adapter)
}

func (s *Store) pendingPath(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), pendingFile)
}

func (s *Store) drainingPath(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), drainingFile)
}

// Write appends a log message to pending JSONL files for all registered adapters.
// This fans out the message so every adapter (e.g., Claude Code, Codex) that has
// registered for this session can independently drain its own copy.
func (s *Store) Write(sessionID string, msg store.LogMessage) error {
	adapters := store.DiscoverAdapters(s.baseDir, sessionID, s.adapter)

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	line := append(data[:len(data):len(data)], '\n')

	for _, adapter := range adapters {
		dir := filepath.Join(s.baseDir, sessionID, adapter)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create adapter dir: %w", err)
		}

		path := filepath.Join(dir, pendingFile)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open pending file: %w", err)
		}
		if _, err := f.Write(line); err != nil {
			_ = f.Close()
			return fmt.Errorf("write message: %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close pending file: %w", err)
		}
	}
	return nil
}

// Drain atomically reads and consumes pending messages for a session.
// Returns at most maxLines messages. Any remaining messages are written
// back to the pending file so they are not lost.
func (s *Store) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	// Ensure own adapter dir exists so writers can discover and fan-out to us.
	if err := os.MkdirAll(s.sessionDir(sessionID), 0o755); err != nil {
		return nil, fmt.Errorf("ensure adapter dir: %w", err)
	}

	if maxLines <= 0 {
		maxLines = defaultMaxLen
	}

	pending := s.pendingPath(sessionID)
	draining := s.drainingPath(sessionID)

	// Atomic rename: pending -> draining
	if err := os.Rename(pending, draining); err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no pending messages
		}
		return nil, fmt.Errorf("rename pending to draining: %w", err)
	}

	// Read ALL entries from the draining file.
	all, err := readJSONL(draining, 0)
	if err != nil {
		// Rollback: rename back so messages aren't lost
		_ = os.Rename(draining, pending)
		return nil, fmt.Errorf("read draining file: %w", err)
	}

	// Successfully read; remove draining file
	_ = os.Remove(draining)

	if len(all) <= maxLines {
		return all, nil
	}

	// Write leftover messages back to pending (appending, since the writer
	// may have already created a new pending.jsonl with fresh data).
	leftover := all[maxLines:]
	if err := s.writeBackLeftover(sessionID, leftover); err != nil {
		// Non-fatal: return what we have but log the issue.
		fmt.Fprintf(os.Stderr, "devtap: write-back leftover: %v\n", err)
	}

	return all[:maxLines], nil
}

// Status returns pending message counts per session.
// It scans baseDir/<session>/<adapter>/pending.jsonl for the store's adapter.
func (s *Store) Status() (map[string]int, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read store dir: %w", err)
	}

	result := make(map[string]int)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		count, err := countLines(s.pendingPath(sessionID))
		if err != nil {
			continue // skip sessions with no pending file
		}
		if count > 0 {
			result[sessionID] = count
		}
	}
	return result, nil
}

// Close is a no-op for file store.
func (s *Store) Close() error {
	return nil
}

// BaseDir returns the base directory of the store.
func (s *Store) BaseDir() string {
	return s.baseDir
}

// writeBackLeftover appends leftover messages to the pending file.
// This preserves messages that exceeded maxLines during a drain.
func (s *Store) writeBackLeftover(sessionID string, messages []store.LogMessage) (retErr error) {
	f, err := os.OpenFile(s.pendingPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open pending for write-back: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("close write-back file: %w", cerr)
		}
	}()

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write leftover: %w", err)
		}
	}
	return nil
}

// readJSONL reads LogMessage entries from a JSONL file.
// If maxLines <= 0, all entries are read (no limit).
func readJSONL(path string, maxLines int) ([]store.LogMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var messages []store.LogMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, scannerInitBuf), scannerMaxBuf)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg store.LogMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, msg)
		if maxLines > 0 && len(messages) >= maxLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return messages, err
	}
	return messages, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count, scanner.Err()
}
