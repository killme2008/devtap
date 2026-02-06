package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/killme2008/devtap/internal/adapter"
	"github.com/killme2008/devtap/internal/session"
)

func TestInstall(t *testing.T) {
	a := New()
	dir := t.TempDir()

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Check .codex/config.toml exists and is valid
	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf(".codex/config.toml not created: %v", err)
	}

	var cfg codexConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		t.Fatalf("invalid TOML: %v", err)
	}

	entry, ok := cfg.MCPServers["devtap"]
	if !ok {
		t.Fatal("missing devtap MCP server entry")
	}
	if len(entry.Args) == 0 || entry.Args[0] != "mcp-serve" {
		t.Errorf("args: got %v, want [mcp-serve]", entry.Args)
	}
}

func TestInstallMergesExisting(t *testing.T) {
	a := New()
	dir := t.TempDir()

	// Create pre-existing .codex/config.toml with another server
	codexDir := filepath.Join(dir, ".codex")
	_ = os.MkdirAll(codexDir, 0o755)
	existing := `[mcp_servers.other]
command = "other-server"
args = ["--flag"]
`
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existing), 0o644)

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	var cfg codexConfig
	_, _ = toml.Decode(string(data), &cfg)

	// Both servers should exist
	if _, ok := cfg.MCPServers["other"]; !ok {
		t.Error("existing 'other' server should be preserved")
	}
	if _, ok := cfg.MCPServers["devtap"]; !ok {
		t.Error("devtap server should be added")
	}
}

func TestDiscoverSessions(t *testing.T) {
	a := New()
	sessions, err := a.DiscoverSessions("/tmp/test-project")
	if err != nil {
		t.Fatalf("DiscoverSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ProjectDir != "/tmp/test-project" {
		t.Errorf("project dir: got %q", sessions[0].ProjectDir)
	}
}

func TestEncodeDir(t *testing.T) {
	got := session.EncodeDir("/tmp/test-project")
	if !strings.HasPrefix(got, "-") {
		t.Errorf("encoded dir should start with separator replacement, got %q", got)
	}
}
