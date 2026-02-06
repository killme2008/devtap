package file

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

func TestWriteAndDrain(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "test",
		Stream:    "stderr",
		Lines:     []string{"error: something failed", "  --> src/main.rs:42"},
	}

	if err := s.Write("session1", msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	messages, err := s.Drain("session1", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Tag != "test" {
		t.Errorf("expected tag 'test', got %q", messages[0].Tag)
	}
	if len(messages[0].Lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(messages[0].Lines))
	}
}

func TestDrainEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	messages, err := s.Drain("nonexistent", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if messages != nil {
		t.Errorf("expected nil, got %v", messages)
	}
}

func TestDrainConsumes(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "test",
		Stream:    "stdout",
		Lines:     []string{"hello"},
	}
	if err := s.Write("s1", msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// First drain should return the message
	messages, err := s.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain 1: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Second drain should return empty (consumed)
	messages, err = s.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain 2: %v", err)
	}
	if messages != nil {
		t.Errorf("expected nil after drain, got %d messages", len(messages))
	}
}

func TestDrainMaxLines(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := 0; i < 10; i++ {
		msg := store.LogMessage{
			Timestamp: time.Now().UTC(),
			Tag:       "test",
			Stream:    "stderr",
			Lines:     []string{"line"},
		}
		if err := s.Write("s1", msg); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	messages, err := s.Drain("s1", 3)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(messages) != 3 {
		t.Errorf("expected 3 messages (maxLines), got %d", len(messages))
	}
}

func TestDrainMaxLinesPreservesLeftover(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Write 10 messages.
	for i := 0; i < 10; i++ {
		msg := store.LogMessage{
			Timestamp: time.Now().UTC(),
			Tag:       "test",
			Stream:    "stderr",
			Lines:     []string{"line"},
		}
		if err := s.Write("s1", msg); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	// Drain 3: should return 3 and preserve the remaining 7.
	batch1, err := s.Drain("s1", 3)
	if err != nil {
		t.Fatalf("Drain 1: %v", err)
	}
	if len(batch1) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(batch1))
	}

	// Drain again: should return the remaining 7.
	batch2, err := s.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain 2: %v", err)
	}
	if len(batch2) != 7 {
		t.Errorf("expected 7 leftover messages, got %d", len(batch2))
	}

	// Drain again: should be empty now.
	batch3, err := s.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain 3: %v", err)
	}
	if batch3 != nil {
		t.Errorf("expected nil after full drain, got %d messages", len(batch3))
	}
}

func TestStatus(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "test",
		Stream:    "stderr",
		Lines:     []string{"error"},
	}
	_ = s.Write("s1", msg)
	_ = s.Write("s1", msg)
	_ = s.Write("s2", msg)

	counts, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if counts["s1"] != 2 {
		t.Errorf("expected s1=2, got %d", counts["s1"])
	}
	if counts["s2"] != 1 {
		t.Errorf("expected s2=1, got %d", counts["s2"])
	}
}

func TestAdapterFanout(t *testing.T) {
	dir := t.TempDir()
	s1, err := New(dir, "claude-code")
	if err != nil {
		t.Fatalf("New s1: %v", err)
	}
	s2, err := New(dir, "codex")
	if err != nil {
		t.Fatalf("New s2: %v", err)
	}

	// Register both adapters (simulates MCP servers starting up).
	// Drain ensures own adapter dir exists for writer discovery.
	_, _ = s1.Drain("s1", 100)
	_, _ = s2.Drain("s1", 100)

	// Write via s1 — should fan-out to both adapter dirs.
	msg := store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       "test",
		Stream:    "stderr",
		Lines:     []string{"build error"},
	}
	if err := s1.Write("s1", msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Both adapters should see the message.
	msgs1, err := s1.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain s1: %v", err)
	}
	if len(msgs1) != 1 {
		t.Fatalf("s1 expected 1 message, got %d", len(msgs1))
	}

	msgs2, err := s2.Drain("s1", 100)
	if err != nil {
		t.Fatalf("Drain s2: %v", err)
	}
	if len(msgs2) != 1 {
		t.Fatalf("s2 expected 1 message, got %d", len(msgs2))
	}

	// After drain, both queues should be empty.
	empty1, _ := s1.Drain("s1", 100)
	empty2, _ := s2.Drain("s1", 100)
	if empty1 != nil {
		t.Errorf("s1 should be empty after drain, got %d", len(empty1))
	}
	if empty2 != nil {
		t.Errorf("s2 should be empty after drain, got %d", len(empty2))
	}
}

func TestConcurrentWriteAndDrain(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "test-adapter")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const numWrites = 100
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numWrites; i++ {
			msg := store.LogMessage{
				Timestamp: time.Now().UTC(),
				Tag:       "test",
				Stream:    "stderr",
				Lines:     []string{"line"},
			}
			_ = s.Write("concurrent", msg)
		}
	}()

	// Drainer goroutine
	var drained int
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			msgs, _ := s.Drain("concurrent", 1000)
			drained += len(msgs)
			time.Sleep(5 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Drain any remaining
	msgs, _ := s.Drain("concurrent", 1000)
	drained += len(msgs)

	// We should have drained at least some messages (exact count depends on timing)
	if drained == 0 {
		t.Error("expected to drain at least some messages")
	}

	// Verify pending file and draining file are cleaned up
	pendingPath := filepath.Join(dir, "concurrent", "test-adapter", pendingFile)
	drainingPath := filepath.Join(dir, "concurrent", "test-adapter", drainingFile)

	if _, err := os.Stat(drainingPath); !os.IsNotExist(err) {
		t.Error("draining file should not exist after drain completes")
	}
	// pending file might or might not exist depending on timing
	_ = pendingPath
}
