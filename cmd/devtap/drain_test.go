package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/killme2008/devtap/internal/mcp"
	"github.com/killme2008/devtap/internal/store"
)

func TestTruncateMessagesNoTruncation(t *testing.T) {
	messages := []store.LogMessage{
		{Tag: "a", Lines: []string{"line1", "line2"}},
		{Tag: "b", Lines: []string{"line3"}},
	}
	result := mcp.TruncateMessages(messages, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if len(result[0].Lines) != 2 || len(result[1].Lines) != 1 {
		t.Error("lines should be unchanged when under limit")
	}
}

func TestTruncateMessagesTruncates(t *testing.T) {
	// Create 2 messages with 60 lines each = 120 total, truncate to 20.
	lines1 := make([]string, 60)
	lines2 := make([]string, 60)
	for i := range lines1 {
		lines1[i] = "msg1-line"
		lines2[i] = "msg2-line"
	}

	messages := []store.LogMessage{
		{Tag: "a", Lines: lines1},
		{Tag: "b", Lines: lines2},
	}
	result := mcp.TruncateMessages(messages, 20)

	// Both messages should be present with truncated lines.
	totalLines := 0
	for _, msg := range result {
		totalLines += len(msg.Lines)
	}
	if totalLines > 20 {
		t.Errorf("total lines %d should not exceed max 20", totalLines)
	}

	// Both messages should have lines (proportional allocation).
	if len(result[0].Lines) == 0 {
		t.Error("first message should have lines after truncation")
	}
	if len(result[1].Lines) == 0 {
		t.Error("second message should have lines after truncation")
	}

	// Tags should be preserved.
	if result[0].Tag != "a" || result[1].Tag != "b" {
		t.Error("tags should be preserved")
	}
}

func TestTruncateMessagesPreservesEmptyLines(t *testing.T) {
	messages := []store.LogMessage{
		{Tag: "a", Lines: []string{"line1"}},
		{Tag: "b"},
		{Tag: "c", Lines: []string{"line2"}},
	}
	result := mcp.TruncateMessages(messages, 10)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[1].Lines != nil {
		t.Error("empty message should remain empty")
	}
}

func TestTruncateMessagesZeroMaxLines(t *testing.T) {
	messages := []store.LogMessage{
		{Tag: "a", Lines: []string{"line1"}},
	}
	result := mcp.TruncateMessages(messages, 0)
	if len(result) != 1 || len(result[0].Lines) != 1 {
		t.Error("maxLines=0 should skip truncation")
	}
}

func TestHandleAutoLoopStopNoErrors(t *testing.T) {
	dir := t.TempDir()
	exitCode := 0
	messages := []store.LogMessage{
		{Tag: "build", ExitCode: &exitCode},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleAutoLoopStop(dir, "test-session", messages, 5)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleAutoLoopStop: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	if output != "{}" {
		t.Errorf("expected '{}' for no errors, got %q", output)
	}
}

func TestHandleAutoLoopStopWithErrors(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "test-session"), 0o755)

	exitCode := 1
	messages := []store.LogMessage{
		{Tag: "build", Lines: []string{"error: something failed"}, ExitCode: &exitCode},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleAutoLoopStop(dir, "test-session", messages, 5)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleAutoLoopStop: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "{}" {
		t.Error("should block when errors present")
	}
	// Should contain "block" decision
	if !contains(output, "block") {
		t.Errorf("output should contain 'block' decision, got %q", output)
	}
}

func TestHandleAutoLoopStopExceedsRetries(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-session"
	_ = os.MkdirAll(filepath.Join(dir, sessionID), 0o755)

	exitCode := 1
	messages := []store.LogMessage{
		{Tag: "build", Lines: []string{"error"}, ExitCode: &exitCode},
	}

	// Exhaust retries.
	maxRetries := 2
	for i := 0; i < maxRetries; i++ {
		old := os.Stdout
		_, w, _ := os.Pipe()
		os.Stdout = w
		_ = handleAutoLoopStop(dir, sessionID, messages, maxRetries)
		_ = w.Close()
		os.Stdout = old
	}

	// Next call should allow stop (return "{}").
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleAutoLoopStop(dir, sessionID, messages, maxRetries)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleAutoLoopStop: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	if output != "{}" {
		t.Errorf("expected '{}' after exhausting retries, got %q", output)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
