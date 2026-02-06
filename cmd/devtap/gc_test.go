package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAllFilesOlderThan(t *testing.T) {
	dir := t.TempDir()
	cutoff := time.Now().Add(-1 * time.Hour)

	// Empty directory — should return true.
	if !allFilesOlderThan(dir, cutoff) {
		t.Error("empty dir should be considered all-older")
	}

	// Create a fresh file — should return false.
	if err := os.WriteFile(filepath.Join(dir, "recent.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if allFilesOlderThan(dir, cutoff) {
		t.Error("dir with recent file should return false")
	}

	// Set file mtime to 2 hours ago — should return true.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "recent.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	if !allFilesOlderThan(dir, cutoff) {
		t.Error("dir with only old files should return true")
	}
}

func TestAllFilesOlderThanRecursive(t *testing.T) {
	dir := t.TempDir()
	cutoff := time.Now().Add(-1 * time.Hour)
	old := time.Now().Add(-2 * time.Hour)

	// Nested: all old files.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(sub, "a.txt"), old, old); err != nil {
		t.Fatal(err)
	}

	if !allFilesOlderThan(dir, cutoff) {
		t.Error("dir with only old nested files should return true")
	}

	// Add a recent nested file.
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if allFilesOlderThan(dir, cutoff) {
		t.Error("dir with recent nested file should return false")
	}
}

func TestGcStoreDir(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	cutoff := time.Now().Add(-24 * time.Hour)

	// Create session with two adapters: one expired, one fresh.
	expiredAdapter := filepath.Join(dir, "session1", "old-adapter")
	freshAdapter := filepath.Join(dir, "session1", "fresh-adapter")
	if err := os.MkdirAll(expiredAdapter, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(freshAdapter, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(expiredAdapter, "pending.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(expiredAdapter, "pending.jsonl"), old, old); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(freshAdapter, "pending.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := gcStoreDir(dir, cutoff); err != nil {
		t.Fatalf("gcStoreDir: %v", err)
	}

	// Expired adapter dir should be gone.
	if _, err := os.Stat(expiredAdapter); !os.IsNotExist(err) {
		t.Error("expired adapter dir should have been removed")
	}
	// Fresh adapter dir should remain.
	if _, err := os.Stat(freshAdapter); err != nil {
		t.Error("fresh adapter dir should still exist")
	}
	// Session dir should still exist (has fresh adapter).
	if _, err := os.Stat(filepath.Join(dir, "session1")); err != nil {
		t.Error("session dir should still exist")
	}
}

func TestGcStoreDirCleansRetryCount(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	cutoff := time.Now().Add(-24 * time.Hour)

	// Create session with one expired adapter + stale retry_count file.
	adapterDir := filepath.Join(dir, "session1", "claude-code")
	if err := os.MkdirAll(adapterDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pf := filepath.Join(adapterDir, "pending.jsonl")
	if err := os.WriteFile(pf, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pf, old, old); err != nil {
		t.Fatal(err)
	}

	retryFile := filepath.Join(dir, "session1", "retry_count")
	if err := os.WriteFile(retryFile, []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(retryFile, old, old); err != nil {
		t.Fatal(err)
	}

	if err := gcStoreDir(dir, cutoff); err != nil {
		t.Fatalf("gcStoreDir: %v", err)
	}

	// Both adapter dir and retry_count should be gone.
	if _, err := os.Stat(adapterDir); !os.IsNotExist(err) {
		t.Error("expired adapter dir should have been removed")
	}
	if _, err := os.Stat(retryFile); !os.IsNotExist(err) {
		t.Error("stale retry_count should have been removed")
	}
	// Session dir should be cleaned up (now empty).
	if _, err := os.Stat(filepath.Join(dir, "session1")); !os.IsNotExist(err) {
		t.Error("empty session dir should have been removed")
	}
}

func TestGcStoreDirNonexistent(t *testing.T) {
	// Non-existent store dir should not error.
	if err := gcStoreDir("/nonexistent/path/devtap", time.Now()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
