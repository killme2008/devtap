package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/killme2008/devtap/internal/adapter"
)

func TestInstall(t *testing.T) {
	a := New()
	dir := t.TempDir()

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatalf("opencode.json not created: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	mcp, ok := cfg["mcp"].(map[string]any)
	if !ok {
		t.Fatal("missing mcp section")
	}

	devtap, ok := mcp["devtap"].(map[string]any)
	if !ok {
		t.Fatal("missing devtap entry")
	}

	if devtap["type"] != "local" {
		t.Errorf("type: got %v, want local", devtap["type"])
	}

	cmd, ok := devtap["command"].([]any)
	if !ok || len(cmd) < 2 {
		t.Fatal("missing or invalid command array")
	}
	if cmd[1] != "mcp-serve" {
		t.Errorf("command[1]: got %v, want mcp-serve", cmd[1])
	}
}

func TestInstallMergesExisting(t *testing.T) {
	a := New()
	dir := t.TempDir()

	// Write pre-existing opencode.json
	existing := `{"mcp": {"other": {"type": "local", "command": ["other-server"]}}, "model": "gpt-4"}`
	_ = os.WriteFile(filepath.Join(dir, configFileName), []byte(existing), 0o644)

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, configFileName))
	var cfg map[string]any
	_ = json.Unmarshal(data, &cfg)

	// Top-level keys preserved
	if cfg["model"] != "gpt-4" {
		t.Error("existing top-level key 'model' should be preserved")
	}

	mcp := cfg["mcp"].(map[string]any)
	if _, ok := mcp["other"]; !ok {
		t.Error("existing 'other' MCP server should be preserved")
	}
	if _, ok := mcp["devtap"]; !ok {
		t.Error("devtap MCP server should be added")
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

func TestName(t *testing.T) {
	a := New()
	if a.Name() != "opencode" {
		t.Errorf("Name: got %q, want opencode", a.Name())
	}
}
