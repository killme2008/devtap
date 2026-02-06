//go:build integration

package greptimedb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/config"
	"github.com/killme2008/devtap/internal/store"
)

const testDatabase = "devtap_test"

// testConfig returns the default local GreptimeDB config.
// Override with DEVTAP_TEST_GREPTIMEDB_ENDPOINT etc. if needed.
func testConfig() config.GreptimeDBConfig {
	return config.GreptimeDBConfig{
		Endpoint:      "127.0.0.1:4001",
		MySQLEndpoint: "127.0.0.1:4002",
		Database:      testDatabase,
	}
}

// newTestStore creates a store and drops the test table for a clean slate.
func newTestStore(t *testing.T) *Store {
	return newTestStoreWithAdapter(t, "test-adapter")
}

// newTestStoreWithAdapter creates a store with a specific adapter name.
// It drops the test table on first call per test for a clean slate.
func newTestStoreWithAdapter(t *testing.T, adapter string) *Store {
	t.Helper()
	baseDir := t.TempDir()
	s, err := New(testConfig(), baseDir, adapter)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Drop existing table for a clean slate.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	s.ensured = false

	t.Cleanup(func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		_, _ = s.db.ExecContext(ctx2, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		s.Close()
	})

	return s
}

func TestIntegrationWriteAndDrain(t *testing.T) {
	s := newTestStore(t)
	sessionID := "integ-test-1"

	// Write two messages with different streams.
	msg1 := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "cargo-check",
		Stream:    "stderr",
		Lines:     []string{"error[E0308]: mismatched types", "  --> src/main.rs:42:5"},
	}
	msg2 := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "cargo-check",
		Stream:    "stdout",
		Lines:     []string{"Compiling myproject v0.1.0"},
	}

	if err := s.Write(sessionID, msg1); err != nil {
		t.Fatalf("Write msg1: %v", err)
	}
	if err := s.Write(sessionID, msg2); err != nil {
		t.Fatalf("Write msg2: %v", err)
	}

	// GreptimeDB needs a moment to flush.
	time.Sleep(2 * time.Second)

	// Drain should return both messages.
	messages, err := s.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify content.
	foundStderr := false
	foundStdout := false
	for _, msg := range messages {
		if msg.Stream == "stderr" && len(msg.Lines) == 2 && msg.Lines[0] == "error[E0308]: mismatched types" {
			foundStderr = true
		}
		if msg.Stream == "stdout" && len(msg.Lines) == 1 && msg.Lines[0] == "Compiling myproject v0.1.0" {
			foundStdout = true
		}
	}
	if !foundStderr {
		t.Error("missing stderr message")
	}
	if !foundStdout {
		t.Error("missing stdout message")
	}

	// Second drain should return empty (watermark advanced).
	messages2, err := s.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain 2: %v", err)
	}
	if len(messages2) != 0 {
		t.Errorf("expected 0 messages on second drain, got %d", len(messages2))
	}
}

func TestIntegrationSameMillisecondDifferentStreams(t *testing.T) {
	s := newTestStore(t)
	sessionID := "integ-test-collision"

	// Write stdout and stderr with the exact same timestamp — this was the
	// collision bug: without stream in PK, the second row would overwrite the first.
	ts := time.Now().UTC()
	msg1 := store.LogMessage{
		Timestamp: ts,
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"error: failure"},
	}
	msg2 := store.LogMessage{
		Timestamp: ts,
		Tag:       "build",
		Stream:    "stdout",
		Lines:     []string{"some output"},
	}
	exitCode := 1
	msg3 := store.LogMessage{
		Timestamp: ts,
		Tag:       "build",
		Stream:    "exit",
		ExitCode:  &exitCode,
	}

	if err := s.Write(sessionID, msg1); err != nil {
		t.Fatalf("Write msg1: %v", err)
	}
	if err := s.Write(sessionID, msg2); err != nil {
		t.Fatalf("Write msg2: %v", err)
	}
	if err := s.Write(sessionID, msg3); err != nil {
		t.Fatalf("Write msg3: %v", err)
	}

	time.Sleep(2 * time.Second)

	messages, err := s.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages (stderr + stdout + exit at same ts), got %d", len(messages))
	}
}

func TestIntegrationDrainMaxLines(t *testing.T) {
	s := newTestStore(t)
	sessionID := "integ-test-maxlines"

	// Write 10 messages.
	for i := 0; i < 10; i++ {
		msg := store.LogMessage{
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Microsecond),
			Tag:       "test",
			Stream:    "stderr",
			Lines:     []string{fmt.Sprintf("line %d", i)},
		}
		if err := s.Write(sessionID, msg); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	// Drain with limit 3.
	messages, err := s.Drain(sessionID, 3)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Drain remaining.
	messages2, err := s.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain 2: %v", err)
	}
	if len(messages2) != 7 {
		t.Errorf("expected 7 remaining messages, got %d", len(messages2))
	}
}

func TestIntegrationDrainSQL(t *testing.T) {
	s := newTestStore(t)
	sessionID := "integ-test-sql"

	msg1 := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"error: something failed"},
	}
	msg2 := store.LogMessage{
		Timestamp: time.Now().UTC().Add(time.Microsecond),
		Tag:       "build",
		Stream:    "stdout",
		Lines:     []string{"info: compiling OK"},
	}

	if err := s.Write(sessionID, msg1); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := s.Write(sessionID, msg2); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Filter only error lines.
	messages, err := s.DrainSQL(sessionID, "content LIKE '%error%'", 100)
	if err != nil {
		t.Fatalf("DrainSQL: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message matching filter, got %d", len(messages))
	}
	if messages[0].Stream != "stderr" {
		t.Errorf("expected stderr, got %q", messages[0].Stream)
	}
}

func TestIntegrationDrainSQLInjectionBlocked(t *testing.T) {
	s := newTestStore(t)

	_, err := s.DrainSQL("test", "1=1; DROP TABLE devtap_logs", 100)
	if err == nil {
		t.Fatal("expected error for SQL injection attempt")
	}
}

func TestIntegrationStatus(t *testing.T) {
	s := newTestStore(t)

	// Write to two sessions.
	for i := 0; i < 3; i++ {
		msg := store.LogMessage{
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Microsecond),
			Tag:       "build",
			Stream:    "stderr",
			Lines:     []string{"line"},
		}
		if err := s.Write("session-a", msg); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"line"},
	}
	if err := s.Write("session-b", msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	counts, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if counts["session-a"] != 3 {
		t.Errorf("session-a: expected 3, got %d", counts["session-a"])
	}
	if counts["session-b"] != 1 {
		t.Errorf("session-b: expected 1, got %d", counts["session-b"])
	}

	// Drain session-a then check status again.
	s.Drain("session-a", 100)
	time.Sleep(time.Second)

	counts2, err := s.Status()
	if err != nil {
		t.Fatalf("Status 2: %v", err)
	}
	if counts2["session-a"] != 0 {
		t.Errorf("session-a after drain: expected 0, got %d", counts2["session-a"])
	}
}

func TestIntegrationStatusExtended(t *testing.T) {
	s := newTestStore(t)

	exitCode := 1
	msgs := []store.LogMessage{
		{Timestamp: time.Now().UTC(), Tag: "cargo", Stream: "stderr", Lines: []string{"error"}},
		{Timestamp: time.Now().UTC().Add(time.Microsecond), Tag: "cargo", Stream: "exit", ExitCode: &exitCode},
		{Timestamp: time.Now().UTC().Add(2 * time.Microsecond), Tag: "cargo", Stream: "stderr", Lines: []string{"another error"}},
	}
	for _, m := range msgs {
		if err := s.Write("stats-session", m); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	time.Sleep(2 * time.Second)

	stats, err := s.StatusExtended()
	if err != nil {
		t.Fatalf("StatusExtended: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one stats entry")
	}

	found := false
	for _, st := range stats {
		if st.SessionID == "stats-session" && st.Tag == "cargo" {
			found = true
			if st.Total < 3 {
				t.Errorf("expected total >= 3, got %d", st.Total)
			}
			if st.Errors < 1 {
				t.Errorf("expected errors >= 1, got %d", st.Errors)
			}
		}
	}
	if !found {
		t.Error("stats entry for stats-session/cargo not found")
	}
}

func TestIntegrationHistory(t *testing.T) {
	s := newTestStore(t)
	sessionID := "history-session"

	exitCode := 1
	msgs := []store.LogMessage{
		{Timestamp: time.Now().UTC(), Tag: "build", Stream: "stderr", Lines: []string{"error: failed"}},
		{Timestamp: time.Now().UTC().Add(time.Microsecond), Tag: "build", Stream: "exit", ExitCode: &exitCode},
	}
	for _, m := range msgs {
		if err := s.Write(sessionID, m); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	time.Sleep(2 * time.Second)

	// Query history: should find the exit event.
	entries, err := s.History(sessionID, "", 1*time.Hour, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one history entry")
	}

	foundExit := false
	for _, e := range entries {
		if e.ExitCode == 1 {
			foundExit = true
		}
	}
	if !foundExit {
		t.Error("history should contain exit code 1 entry")
	}
}

func TestIntegrationHistoryWithTagFilter(t *testing.T) {
	s := newTestStore(t)
	sessionID := "history-tag-session"

	exitCode := 0
	if err := s.Write(sessionID, store.LogMessage{
		Timestamp: time.Now().UTC(), Tag: "cargo", Stream: "exit", ExitCode: &exitCode,
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	exitCode2 := 1
	if err := s.Write(sessionID, store.LogMessage{
		Timestamp: time.Now().UTC().Add(time.Microsecond), Tag: "npm", Stream: "exit", ExitCode: &exitCode2,
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	entries, err := s.History(sessionID, "npm", 1*time.Hour, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, e := range entries {
		if e.Tag != "npm" {
			t.Errorf("expected only 'npm' tag entries, got %q", e.Tag)
		}
	}
}

func TestIntegrationHistoryFallbackToAllStreams(t *testing.T) {
	s := newTestStore(t)
	sessionID := "history-fallback"

	// Write only stderr (no exit stream) — history should fallback and return these.
	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "lint",
		Stream:    "stderr",
		Lines:     []string{"warning: unused variable"},
	}
	if err := s.Write(sessionID, msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	entries, err := s.History(sessionID, "", 1*time.Hour, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) == 0 {
		t.Error("history fallback should return non-exit entries when no exit entries exist")
	}
}

func TestIntegrationWatermarkPersistence(t *testing.T) {
	s := newTestStore(t)
	sessionID := "watermark-test"

	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"error line"},
	}
	if err := s.Write(sessionID, msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Drain to advance watermark.
	messages, err := s.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Watermark file should exist.
	wmPath := s.watermarkPath(sessionID)
	if _, err := os.Stat(wmPath); os.IsNotExist(err) {
		t.Fatal("watermark file should exist after drain")
	}

	// Create a new store instance pointing to the same baseDir —
	// watermark should persist and second drain should return empty.
	s2, err := New(testConfig(), s.baseDir, "test-adapter")
	if err != nil {
		t.Fatalf("New s2: %v", err)
	}
	defer s2.Close()

	messages2, err := s2.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain s2: %v", err)
	}
	if len(messages2) != 0 {
		t.Errorf("expected 0 messages after watermark persists, got %d", len(messages2))
	}
}

func TestIntegrationEnsureTableIdempotent(t *testing.T) {
	s := newTestStore(t)

	// ensureTable should be idempotent (CREATE IF NOT EXISTS).
	if err := s.ensureTable(); err != nil {
		t.Fatalf("ensureTable 1: %v", err)
	}
	s.ensured = false
	if err := s.ensureTable(); err != nil {
		t.Fatalf("ensureTable 2: %v", err)
	}
}

func TestIntegrationAdapterFanout(t *testing.T) {
	// Both stores share the same baseDir so DiscoverAdapters can find both.
	baseDir := t.TempDir()
	s1, err := New(testConfig(), baseDir, "claude-code")
	if err != nil {
		t.Fatalf("New s1: %v", err)
	}

	// Drop table for clean slate.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s1.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	s1.ensured = false

	s2, err := New(testConfig(), baseDir, "codex")
	if err != nil {
		t.Fatalf("New s2: %v", err)
	}

	t.Cleanup(func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		_, _ = s1.db.ExecContext(ctx2, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		s1.Close()
		s2.Close()
	})

	sessionID := "fanout-test"

	// Register both adapters by draining (creates adapter dirs).
	s1.Drain(sessionID, 100)
	s2.Drain(sessionID, 100)

	// Write a single message via s1 — should fan-out to both adapters.
	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"error: something failed"},
	}
	if err := s1.Write(sessionID, msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Both adapters should see the message.
	msgs1, err := s1.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain s1: %v", err)
	}
	if len(msgs1) != 1 {
		t.Fatalf("s1 expected 1 message, got %d", len(msgs1))
	}
	if msgs1[0].Adapter != "claude-code" {
		t.Errorf("s1 message adapter: got %q, want %q", msgs1[0].Adapter, "claude-code")
	}

	msgs2, err := s2.Drain(sessionID, 100)
	if err != nil {
		t.Fatalf("Drain s2: %v", err)
	}
	if len(msgs2) != 1 {
		t.Fatalf("s2 expected 1 message, got %d", len(msgs2))
	}
	if msgs2[0].Adapter != "codex" {
		t.Errorf("s2 message adapter: got %q, want %q", msgs2[0].Adapter, "codex")
	}
}

func TestIntegrationPing(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
