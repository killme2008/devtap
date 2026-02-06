package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Store.Backend != "file" {
		t.Errorf("default backend: got %q, want %q", cfg.Store.Backend, "file")
	}
	if cfg.Store.GreptimeDB.Endpoint != "127.0.0.1:4001" {
		t.Errorf("default gRPC endpoint: got %q", cfg.Store.GreptimeDB.Endpoint)
	}
	if cfg.Store.GreptimeDB.MySQLEndpoint != "127.0.0.1:4002" {
		t.Errorf("default MySQL endpoint: got %q", cfg.Store.GreptimeDB.MySQLEndpoint)
	}
	if cfg.Store.GreptimeDB.Database != "devtap" {
		t.Errorf("default database: got %q", cfg.Store.GreptimeDB.Database)
	}
}

func TestLoadNonexistent(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("LoadFrom nonexistent: %v", err)
	}
	if cfg.Store.Backend != "file" {
		t.Errorf("should fall back to defaults, got backend %q", cfg.Store.Backend)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
[store]
backend = "greptimedb"

[store.greptimedb]
endpoint = "10.0.0.1:4001"
mysql_endpoint = "10.0.0.1:4002"
database = "mydb"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Store.Backend != "greptimedb" {
		t.Errorf("backend: got %q, want greptimedb", cfg.Store.Backend)
	}
	if cfg.Store.GreptimeDB.Endpoint != "10.0.0.1:4001" {
		t.Errorf("endpoint: got %q", cfg.Store.GreptimeDB.Endpoint)
	}
	if cfg.Store.GreptimeDB.MySQLEndpoint != "10.0.0.1:4002" {
		t.Errorf("mysql_endpoint: got %q", cfg.Store.GreptimeDB.MySQLEndpoint)
	}
	if cfg.Store.GreptimeDB.Database != "mydb" {
		t.Errorf("database: got %q", cfg.Store.GreptimeDB.Database)
	}
}

func TestLoadPartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Only override backend, rest should keep defaults
	content := `
[store]
backend = "greptimedb"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Store.Backend != "greptimedb" {
		t.Errorf("backend: got %q", cfg.Store.Backend)
	}
	// Defaults should be preserved
	if cfg.Store.GreptimeDB.Endpoint != "127.0.0.1:4001" {
		t.Errorf("endpoint should be default, got %q", cfg.Store.GreptimeDB.Endpoint)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(path, []byte("invalid [[ toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestAuthFromEnv(t *testing.T) {
	cfg := Default()

	t.Setenv("DEVTAP_GREPTIMEDB_AUTH_TOKEN", "test-token")
	t.Setenv("DEVTAP_GREPTIMEDB_USERNAME", "user1")
	t.Setenv("DEVTAP_GREPTIMEDB_PASSWORD", "pass1")

	if cfg.Store.GreptimeDB.AuthToken() != "test-token" {
		t.Error("AuthToken should read from env")
	}
	if cfg.Store.GreptimeDB.Username() != "user1" {
		t.Error("Username should read from env")
	}
	if cfg.Store.GreptimeDB.Password() != "pass1" {
		t.Error("Password should read from env")
	}
}
