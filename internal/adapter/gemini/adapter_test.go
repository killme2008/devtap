package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/killme2008/devtap/internal/adapter"
)

func TestName(t *testing.T) {
	a := New()
	if a.Name() != "gemini" {
		t.Errorf("Name: got %q, want gemini", a.Name())
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

func TestInstall(t *testing.T) {
	a := New()
	dir := t.TempDir()

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, configFilePath))
	if err != nil {
		t.Fatalf(".gemini/settings.json not created: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("missing mcpServers section")
	}

	devtap, ok := servers["devtap"].(map[string]any)
	if !ok {
		t.Fatal("missing devtap entry")
	}

	args, ok := devtap["args"].([]any)
	if !ok || len(args) < 1 {
		t.Fatal("missing or invalid args array")
	}
	if args[0] != "mcp-serve" {
		t.Errorf("args[0]: got %v, want mcp-serve", args[0])
	}
}

func TestInstallMergesExisting(t *testing.T) {
	a := New()
	dir := t.TempDir()

	// Write pre-existing .gemini/settings.json
	geminiDir := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"mcpServers": {"other": {"command": "other-server"}}, "theme": "dark"}`
	if err := os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, configFilePath))
	var cfg map[string]any
	_ = json.Unmarshal(data, &cfg)

	// Top-level keys preserved
	if cfg["theme"] != "dark" {
		t.Error("existing top-level key 'theme' should be preserved")
	}

	servers := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("existing 'other' MCP server should be preserved")
	}
	if _, ok := servers["devtap"]; !ok {
		t.Error("devtap MCP server should be added")
	}
}
