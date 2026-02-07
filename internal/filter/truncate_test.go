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

func TestTruncateWithRatio_TailBiased(t *testing.T) {
	// 20 distinct lines, maxLines=10, tailRatio=0.8 → 2 head + 8 tail
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	result := TruncateWithRatio(lines, 10, 0.8)

	// 2 head + 1 omission + 8 tail = 11 entries
	if len(result) != 11 {
		t.Fatalf("expected 11 lines, got %d: %v", len(result), result)
	}
	// First 2 should be head
	if result[0] != "line-0" || result[1] != "line-1" {
		t.Errorf("head: got %q, %q", result[0], result[1])
	}
	// Omission marker
	if result[2] != "... (10 lines omitted)" {
		t.Errorf("omission: got %q", result[2])
	}
	// Last 8 should be tail
	if result[3] != "line-12" || result[10] != "line-19" {
		t.Errorf("tail: got first=%q last=%q", result[3], result[10])
	}
}

func TestTruncateWithRatio_AllHead(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	// tailRatio=0.0 → still gets at least 1 tail line
	result := TruncateWithRatio(lines, 4, 0.0)
	// 3 head + 1 omission + 1 tail = 5
	if len(result) != 5 {
		t.Fatalf("expected 5 lines, got %d: %v", len(result), result)
	}
	if result[4] != "line-9" {
		t.Errorf("last line should be tail, got %q", result[4])
	}
}

func TestTruncateWithRatio_AllTail(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	// tailRatio=1.0 → still gets at least 1 head line
	result := TruncateWithRatio(lines, 4, 1.0)
	// 1 head + 1 omission + 3 tail = 5
	if len(result) != 5 {
		t.Fatalf("expected 5 lines, got %d: %v", len(result), result)
	}
	if result[0] != "line-0" {
		t.Errorf("first line should be head, got %q", result[0])
	}
	if result[4] != "line-9" {
		t.Errorf("last line should be tail, got %q", result[4])
	}
}

func TestTruncateWithRatio_NoTruncationNeeded(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := TruncateWithRatio(lines, 10, 0.8)
	if len(result) != 3 {
		t.Errorf("expected 3 lines (no truncation), got %d", len(result))
	}
}

func TestTruncateWithRatio_SingleMax(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := TruncateWithRatio(lines, 1, 0.8)
	// maxLines=1: return only the last line, no omission marker
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if result[0] != "c" {
		t.Errorf("expected last line %q, got %q", "c", result[0])
	}
}

func TestTruncateSingleMax_Legacy(t *testing.T) {
	// Truncate (50/50 ratio) with maxLines=1 should also return exactly 1 line.
	lines := []string{"x", "y", "z"}
	result := Truncate(lines, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if result[0] != "z" {
		t.Errorf("expected last line %q, got %q", "z", result[0])
	}
}
