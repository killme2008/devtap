package filter

import (
	"fmt"
	"testing"
)

func TestTruncateNoOp(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := Truncate(lines, 10)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
}

func TestTruncateZeroMax(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := Truncate(lines, 0)
	if len(result) != 3 {
		t.Errorf("expected 3 lines (no truncation), got %d", len(result))
	}
}

func TestTruncateWithDuplicates(t *testing.T) {
	// 20 identical lines get deduped first to 2 lines ("line" + "(repeated 20 times)")
	// which is under maxLines=6, so no truncation needed.
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}

	result := Truncate(lines, 6)
	if len(result) != 2 {
		t.Errorf("expected 2 lines after dedup, got %d: %v", len(result), result)
	}
}

func TestTruncateDistinctLines(t *testing.T) {
	// 20 distinct lines should be truncated to head + omission + tail
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	result := Truncate(lines, 6)

	// Should have 3 head + 1 omission + 3 tail = 7
	found := false
	for _, line := range result {
		if line == "... (14 lines omitted)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected omission marker in result: %v", result)
	}
	if len(result) > 7 {
		t.Errorf("expected <= 7 lines, got %d", len(result))
	}
}

func TestDedupConsecutive(t *testing.T) {
	lines := []string{"a", "a", "a", "b", "b", "c"}
	result := dedup(lines)

	expected := []string{"a", "(repeated 3 times)", "b", "(repeated 2 times)", "c"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d lines, got %d: %v", len(expected), len(result), result)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("line %d: got %q, want %q", i, result[i], want)
		}
	}
}

func TestDedupNoRepeats(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := dedup(lines)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
}

func TestDedupEmpty(t *testing.T) {
	result := dedup(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 lines, got %d", len(result))
	}
}

func TestTruncateSingleLine(t *testing.T) {
	result := Truncate([]string{"only"}, 1)
	if len(result) != 1 || result[0] != "only" {
		t.Errorf("unexpected result: %v", result)
	}
}
