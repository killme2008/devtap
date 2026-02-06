package store

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestDiscoverAdaptersNoSessionDir(t *testing.T) {
	dir := t.TempDir()
	// Session dir doesn't exist — should return only self.
	result := DiscoverAdapters(dir, "nonexistent", "claude-code")
	if len(result) != 1 || result[0] != "claude-code" {
		t.Errorf("expected [claude-code], got %v", result)
	}
}

func TestDiscoverAdaptersEmptySessionDir(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "s1")
	if err := os.MkdirAll(sessionPath, 0o755); err != nil {
		t.Fatal(err)
	}

	result := DiscoverAdapters(dir, "s1", "claude-code")
	if len(result) != 1 || result[0] != "claude-code" {
		t.Errorf("expected [claude-code], got %v", result)
	}
}

func TestDiscoverAdaptersMultiple(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "s1")
	if err := os.MkdirAll(filepath.Join(sessionPath, "claude-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sessionPath, "codex"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := DiscoverAdapters(dir, "s1", "claude-code")
	sort.Strings(result)
	if len(result) != 2 || result[0] != "claude-code" || result[1] != "codex" {
		t.Errorf("expected [claude-code codex], got %v", result)
	}
}

func TestDiscoverAdaptersIgnoresFiles(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "s1")
	if err := os.MkdirAll(filepath.Join(sessionPath, "claude-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a regular file that should be ignored.
	if err := os.WriteFile(filepath.Join(sessionPath, "retry_count"), []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverAdapters(dir, "s1", "claude-code")
	if len(result) != 1 || result[0] != "claude-code" {
		t.Errorf("expected [claude-code], got %v", result)
	}
}

func TestDiscoverAdaptersSelfAlwaysIncluded(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "s1")
	// Only codex dir exists, but self is "claude-code".
	if err := os.MkdirAll(filepath.Join(sessionPath, "codex"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := DiscoverAdapters(dir, "s1", "claude-code")
	sort.Strings(result)
	if len(result) != 2 || result[0] != "claude-code" || result[1] != "codex" {
		t.Errorf("expected [claude-code codex], got %v", result)
	}
}

func TestEnsureAdapterDir(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureAdapterDir(dir, "s1", "claude-code"); err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(dir, "s1", "claude-code")
	info, err := os.Stat(expected)
	if err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}
