package capture

import (
	"testing"
	"time"

	filestore "github.com/killme2008/devtap/internal/store/file"
)

func TestRunSuccess(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := New(s, "test-session", "echo", nil)
	result, err := runner.Run([]string{"echo", "hello world"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// Drain and verify output was captured
	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Should have at least the stdout batch and exit code message
	if len(messages) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(messages))
	}

	// Check that exit code message exists
	var hasExit bool
	for _, msg := range messages {
		if msg.ExitCode != nil && *msg.ExitCode == 0 {
			hasExit = true
		}
	}
	if !hasExit {
		t.Error("expected exit code message")
	}
}

func TestRunFailure(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := New(s, "test-session", "false", nil)
	result, err := runner.Run([]string{"false"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRunWithFilter(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	filter := func(line string) bool {
		return line == "keep"
	}

	runner := New(s, "test-session", "test", filter)
	result, err := runner.Run([]string{"printf", "keep\nskip\nkeep\n"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// Allow some time for flush
	time.Sleep(50 * time.Millisecond)

	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Check that only "keep" lines were stored
	for _, msg := range messages {
		for _, line := range msg.Lines {
			if line != "keep" {
				t.Errorf("unexpected line in filtered output: %q", line)
			}
		}
	}
}

func TestRunNoCommand(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := New(s, "test-session", "test", nil)
	_, err = runner.Run(nil)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRunStderr(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := New(s, "test-session", "sh", nil)
	result, err := runner.Run([]string{"sh", "-c", "echo error >&2"})
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

	// Should have captured stderr
	var hasStderr bool
	for _, msg := range messages {
		if msg.Stream == "stderr" && len(msg.Lines) > 0 {
			hasStderr = true
		}
	}
	if !hasStderr {
		t.Error("expected stderr capture")
	}
}

func TestRunExitCodeStored(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir, "test")
	if err != nil {
		t.Fatalf("New store: %v", err)
	}

	runner := New(s, "test-session", "sh", nil)
	result, err := runner.Run([]string{"sh", "-c", "exit 42"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}

	messages, err := s.Drain("test-session", 100)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var found bool
	for _, msg := range messages {
		if msg.ExitCode != nil {
			if *msg.ExitCode != 42 {
				t.Errorf("stored exit code: got %d, want 42", *msg.ExitCode)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected exit code message")
	}
}
