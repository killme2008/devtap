package store

import "testing"

func TestRetryTracker(t *testing.T) {
	dir := t.TempDir()
	tracker := NewRetryTracker(dir)

	// Initial count should be 0
	count, err := tracker.Get("s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Increment
	n, err := tracker.Increment("s1")
	if err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}

	n, err = tracker.Increment("s1")
	if err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}

	// Verify via Get
	count, err = tracker.Get("s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	// Reset
	if err := tracker.Reset("s1"); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	count, err = tracker.Get("s1")
	if err != nil {
		t.Fatalf("Get after reset: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 after reset, got %d", count)
	}
}

func TestRetryTrackerResetNonexistent(t *testing.T) {
	dir := t.TempDir()
	tracker := NewRetryTracker(dir)

	// Should not error on nonexistent session
	if err := tracker.Reset("nonexistent"); err != nil {
		t.Errorf("Reset nonexistent: %v", err)
	}
}
