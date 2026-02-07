package main

import (
	"testing"

	"github.com/killme2008/devtap/internal/config"
)

func TestOpenStoreStrictFileBackend(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()

	s, err := openStoreStrict(cfg, "file", dir, "claude-code")
	if err != nil {
		t.Fatalf("openStoreStrict(file): %v", err)
	}
	_ = s.Close()
}

func TestOpenStoreStrictEmptyBackend(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()

	s, err := openStoreStrict(cfg, "", dir, "claude-code")
	if err != nil {
		t.Fatalf("openStoreStrict(''): %v", err)
	}
	_ = s.Close()
}

func TestOpenStoreStrictUnknownBackend(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()

	_, err := openStoreStrict(cfg, "redis", dir, "claude-code")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestOpenStoreStrictGreptimeDBNoFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	// Point to an unreachable endpoint so New or Ping fails.
	cfg.Store.GreptimeDB.Endpoint = "127.0.0.1:19999"
	cfg.Store.GreptimeDB.MySQLEndpoint = "127.0.0.1:19998"

	_, err := openStoreStrict(cfg, "greptimedb", dir, "claude-code")
	if err == nil {
		t.Fatal("expected error when greptimedb is unreachable, should not fall back to file store")
	}
}
