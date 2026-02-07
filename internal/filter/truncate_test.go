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

func TestTruncateTailBiased(t *testing.T) {
	// 20 distinct lines, maxLines=10 → with 0.8 tail ratio: 2 head + 8 tail
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	result := Truncate(lines, 10)

	// 2 head + 1 omission + 8 tail = 11 entries
	if len(result) != 11 {
		t.Fatalf("expected 11 lines, got %d: %v", len(result), result)
	}
	if result[0] != "line-0" || result[1] != "line-1" {
		t.Errorf("head: got %q, %q", result[0], result[1])
	}
	if result[2] != "... (10 lines omitted)" {
		t.Errorf("omission: got %q", result[2])
	}
	if result[3] != "line-12" || result[10] != "line-19" {
		t.Errorf("tail: got first=%q last=%q", result[3], result[10])
	}
}

func TestTruncateDistinctLines(t *testing.T) {
	// 20 distinct lines, maxLines=6 → with 0.8 tail ratio: 2 head + 4 tail (int(6*0.8)=4)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	result := Truncate(lines, 6)

	// head=2 (6-4), tail=4 → 2 head + 1 omission + 4 tail = 7
	if len(result) != 7 {
		t.Fatalf("expected 7 lines, got %d: %v", len(result), result)
	}
	if result[2] != "... (14 lines omitted)" {
		t.Errorf("expected omission marker, got %q", result[2])
	}
	// Tail should end with the last line
	if result[6] != "line-19" {
		t.Errorf("last line should be line-19, got %q", result[6])
	}
}

func TestTruncateSingleLine(t *testing.T) {
	result := Truncate([]string{"only"}, 1)
	if len(result) != 1 || result[0] != "only" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestTruncateSingleMax(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := Truncate(lines, 1)
	// maxLines=1: return only the last line, no omission marker
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if result[0] != "c" {
		t.Errorf("expected last line %q, got %q", "c", result[0])
	}
}

func TestTruncateSingleMaxWithDuplicates(t *testing.T) {
	// maxLines=1 with repeated lines should return the last real line,
	// not a "(repeated N times)" marker from dedup.
	lines := []string{"error: foo", "error: foo", "error: foo"}
	result := Truncate(lines, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if result[0] != "error: foo" {
		t.Errorf("expected last real line, got %q", result[0])
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
