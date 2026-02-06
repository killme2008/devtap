package capture

import (
	"testing"
	"time"

	filestore "github.com/killme2008/devtap/internal/store/file"
)

func TestLongRunnerBasic(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := NewLongRunner(s, "test-session", "echo", nil, 100*time.Millisecond)
	result, err := runner.Run([]string{"echo", "hello from longrun"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// Wait a bit for final flush
	time.Sleep(150 * time.Millisecond)

	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if len(messages) < 1 {
		t.Error("expected at least 1 message")
	}
}

func TestLongRunnerDebounce(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	// Use a longer debounce so output is batched
	runner := NewLongRunner(s, "test-session", "seq", nil, 500*time.Millisecond)
	result, err := runner.Run([]string{"sh", "-c", "for i in 1 2 3 4 5; do echo line-$i; done"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// All lines should be in fewer messages due to debounce batching
	var totalLines int
	for _, msg := range messages {
		totalLines += len(msg.Lines)
	}

	if totalLines < 5 {
		t.Errorf("expected at least 5 lines captured, got %d", totalLines)
	}
}

func TestLongRunnerWithFilter(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	filter := func(line string) bool {
		return line == "keep"
	}

	runner := NewLongRunner(s, "test-session", "test", filter, 100*time.Millisecond)
	result, err := runner.Run([]string{"printf", "keep\nskip\nkeep\n"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	time.Sleep(150 * time.Millisecond)

	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	for _, msg := range messages {
		for _, line := range msg.Lines {
			if line != "keep" {
				t.Errorf("unexpected line in filtered output: %q", line)
			}
		}
	}
}

func TestLongRunnerExitCode(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := NewLongRunner(s, "test-session", "sh", nil, 100*time.Millisecond)
	result, err := runner.Run([]string{"sh", "-c", "exit 42"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestLongRunnerNoCommand(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := NewLongRunner(s, "test-session", "test", nil, 100*time.Millisecond)
	_, err = runner.Run(nil)
	if err == nil {
		t.Error("expected error for empty command")
	}
}
