package mcp

import (
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

func intPtr(v int) *int { return &v }

func TestCollapseSuccessful_ExitZero(t *testing.T) {
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "go-build", Stream: "stdout", Lines: []string{"line1", "line2", "line3"}},
		{Timestamp: ts, Tag: "go-build", Stream: "exit", ExitCode: intPtr(0)},
	}

	result := CollapseSuccessful(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result[0].Lines))
	}
	want := "(3 lines of output omitted — build succeeded)"
	if result[0].Lines[0] != want {
		t.Errorf("got %q, want %q", result[0].Lines[0], want)
	}
	if result[0].Tag != "go-build" {
		t.Errorf("tag: got %q, want %q", result[0].Tag, "go-build")
	}
	if result[0].ExitCode == nil || *result[0].ExitCode != 0 {
		t.Error("exit code should be preserved as 0")
	}
}

func TestCollapseSuccessful_NonZeroPreserved(t *testing.T) {
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "cargo", Stream: "stderr", Lines: []string{"error[E0308]: mismatched types"}},
		{Timestamp: ts, Tag: "cargo", Stream: "exit", ExitCode: intPtr(1)},
	}

	result := CollapseSuccessful(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (unchanged), got %d", len(result))
	}
	if result[0].Lines[0] != "error[E0308]: mismatched types" {
		t.Error("error message should be preserved")
	}
}

func TestCollapseSuccessful_NoExitCodePreserved(t *testing.T) {
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "dev-server", Stream: "stdout", Lines: []string{"listening on :8080"}},
	}

	result := CollapseSuccessful(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Lines[0] != "listening on :8080" {
		t.Error("message without exit code should be preserved")
	}
}

func TestCollapseSuccessful_MixedTags(t *testing.T) {
	ts := time.Now()
	messages := []store.LogMessage{
		// Successful build
		{Timestamp: ts, Tag: "go-build", Stream: "stdout", Lines: []string{"compiling...", "done"}},
		{Timestamp: ts, Tag: "go-build", Stream: "exit", ExitCode: intPtr(0)},
		// Failed test
		{Timestamp: ts, Tag: "go-test", Stream: "stderr", Lines: []string{"FAIL TestFoo"}},
		{Timestamp: ts, Tag: "go-test", Stream: "exit", ExitCode: intPtr(1)},
	}

	result := CollapseSuccessful(messages)

	// go-build collapsed to 1, go-test preserved as 2
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// First should be collapsed summary
	if len(result[0].Lines) != 1 || result[0].Tag != "go-build" {
		t.Errorf("first message should be collapsed go-build, got tag=%q lines=%v", result[0].Tag, result[0].Lines)
	}

	// Remaining should be go-test preserved
	if result[1].Lines[0] != "FAIL TestFoo" {
		t.Error("go-test error should be preserved")
	}
}

func TestCollapseSuccessful_EmptyMessages(t *testing.T) {
	result := CollapseSuccessful(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestCollapseSuccessful_ExitZeroNoLines(t *testing.T) {
	// Exit code 0 but zero output lines — nothing to collapse
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "build", Stream: "exit", ExitCode: intPtr(0)},
	}

	result := CollapseSuccessful(messages)

	// No lines to collapse, so the exit message is kept as-is
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Lines) != 0 {
		t.Errorf("expected 0 lines, got %d", len(result[0].Lines))
	}
}

func TestCollapseSuccessful_SameTagFailThenSuccess(t *testing.T) {
	// Same tag appears twice: first run fails, second run succeeds.
	// The failure must be preserved; only the success run is collapsed.
	ts := time.Now()
	messages := []store.LogMessage{
		// Run 1: failure
		{Timestamp: ts, Tag: "make", Stream: "stderr", Lines: []string{"error: undefined reference"}},
		{Timestamp: ts, Tag: "make", Stream: "exit", ExitCode: intPtr(2)},
		// Run 2: success after fix
		{Timestamp: ts, Tag: "make", Stream: "stdout", Lines: []string{"compiling...", "linking...", "done"}},
		{Timestamp: ts, Tag: "make", Stream: "exit", ExitCode: intPtr(0)},
	}

	result := CollapseSuccessful(messages)

	// Run 1 preserved (2 messages), run 2 collapsed (1 summary)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(result), result)
	}

	// First two: the failed run, unchanged
	if result[0].Lines[0] != "error: undefined reference" {
		t.Errorf("failed run output should be preserved, got %q", result[0].Lines[0])
	}
	if result[1].ExitCode == nil || *result[1].ExitCode != 2 {
		t.Errorf("failed run exit code should be 2, got %v", result[1].ExitCode)
	}

	// Third: collapsed success summary
	want := "(3 lines of output omitted — build succeeded)"
	if len(result[2].Lines) != 1 || result[2].Lines[0] != want {
		t.Errorf("success run should be collapsed, got %v", result[2].Lines)
	}
}

func TestCollapseSuccessful_SameTagSuccessThenFail(t *testing.T) {
	// Reverse order: success first, then failure. Failure must be preserved.
	ts := time.Now()
	messages := []store.LogMessage{
		// Run 1: success
		{Timestamp: ts, Tag: "build", Stream: "stdout", Lines: []string{"ok"}},
		{Timestamp: ts, Tag: "build", Stream: "exit", ExitCode: intPtr(0)},
		// Run 2: failure
		{Timestamp: ts, Tag: "build", Stream: "stderr", Lines: []string{"FAIL"}},
		{Timestamp: ts, Tag: "build", Stream: "exit", ExitCode: intPtr(1)},
	}

	result := CollapseSuccessful(messages)

	// Run 1 collapsed (1 summary), run 2 preserved (2 messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(result), result)
	}

	// First: collapsed success
	if len(result[0].Lines) != 1 {
		t.Fatalf("success run should be 1 summary line, got %d", len(result[0].Lines))
	}

	// Last two: preserved failure
	if result[1].Lines[0] != "FAIL" {
		t.Errorf("failure output should be preserved, got %q", result[1].Lines[0])
	}
	if result[2].ExitCode == nil || *result[2].ExitCode != 1 {
		t.Error("failure exit code should be preserved")
	}
}

func TestCollapseSuccessful_InterleavedTags(t *testing.T) {
	// Two tags interleaved: make fails, test succeeds.
	// Output order should remain aligned with original global message ordering.
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "make", Stream: "stderr", Lines: []string{"err1"}},
		{Timestamp: ts, Tag: "test", Stream: "stdout", Lines: []string{"ok1"}},
		{Timestamp: ts, Tag: "make", Stream: "exit", ExitCode: intPtr(1)},
		{Timestamp: ts, Tag: "test", Stream: "exit", ExitCode: intPtr(0)},
	}

	result := CollapseSuccessful(messages)

	// make: 2 messages preserved, test: 1 collapsed summary
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(result), result)
	}
	// First: make output preserved.
	if result[0].Lines[0] != "err1" {
		t.Errorf("make error should be preserved, got %q", result[0].Lines[0])
	}
	// Second: test summary replaces the first test line position.
	if result[1].ExitCode == nil || *result[1].ExitCode != 0 {
		t.Error("test run should be collapsed with exit code 0")
	}
	if len(result[1].Lines) != 1 {
		t.Errorf("collapsed test should have 1 summary line, got %d", len(result[1].Lines))
	}
	// Third: make exit preserved at its original relative position.
	if result[2].ExitCode == nil || *result[2].ExitCode != 1 {
		t.Error("make exit code 1 should be preserved")
	}
}

func TestCollapseSuccessful_EmptyTagDefaultsBuild(t *testing.T) {
	ts := time.Now()
	messages := []store.LogMessage{
		{Timestamp: ts, Tag: "", Stream: "stdout", Lines: []string{"output"}},
		{Timestamp: ts, Tag: "", Stream: "exit", ExitCode: intPtr(0)},
	}

	result := CollapseSuccessful(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (collapsed), got %d", len(result))
	}
	want := "(1 line of output omitted — build succeeded)"
	if result[0].Lines[0] != want {
		t.Errorf("got %q, want %q", result[0].Lines[0], want)
	}
}
