package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/killme2008/devtap/internal/adapter"
)

func TestWriteMCPConfig(t *testing.T) {
	dir := t.TempDir()
	if err := writeMCPConfig(dir); err != nil {
		t.Fatalf("writeMCPConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("missing mcpServers")
	}

	devtap, ok := servers["devtap"].(map[string]any)
	if !ok {
		t.Fatal("missing devtap entry")
	}

	if devtap["command"] == nil {
		t.Error("missing command")
	}

	args, ok := devtap["args"].([]any)
	if !ok || len(args) == 0 {
		t.Fatal("missing args")
	}
	if args[0] != "mcp-serve" {
		t.Errorf("args[0]: got %v, want mcp-serve", args[0])
	}
}

func TestWriteMCPConfigMergesExisting(t *testing.T) {
	dir := t.TempDir()

	// Write an existing .mcp.json with another server
	existing := map[string]any{
		"mcpServers": map[string]any{
			"other-server": map[string]any{
				"command": "other-bin",
				"args":    []any{"serve"},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	if err := writeMCPConfig(dir); err != nil {
		t.Fatalf("writeMCPConfig: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	servers := config["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Error("existing MCP server 'other-server' was lost during merge")
	}
	if _, ok := servers["devtap"]; !ok {
		t.Error("devtap entry was not added")
	}
}

func TestUpsertMatcherGroup(t *testing.T) {
	// Create initial group
	list := upsertMatcherGroup(nil, "", "devtap drain --event Stop")
	if len(list) != 1 {
		t.Fatalf("expected 1 group, got %d", len(list))
	}

	// Verify structure
	group := list[0].(map[string]any)
	hooks := group["hooks"].([]any)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	hook := hooks[0].(map[string]any)
	if hook["type"] != "command" {
		t.Errorf("type: got %v, want command", hook["type"])
	}

	// Upsert again should replace, not duplicate
	list = upsertMatcherGroup(list, "", "devtap drain --event Stop --updated")
	if len(list) != 1 {
		t.Errorf("expected 1 group after upsert, got %d", len(list))
	}

	// Non-devtap hooks should be preserved
	existingWithOther := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "echo hello"},
			},
		},
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "devtap drain --old"},
			},
		},
	}
	list = upsertMatcherGroup(existingWithOther, "", "devtap drain --new")
	if len(list) != 2 {
		t.Errorf("expected 2 groups (other + devtap), got %d", len(list))
	}
}

func TestIsDevtapMatcherGroup(t *testing.T) {
	devtapGroup := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "devtap drain --event Stop"},
		},
	}
	if !isDevtapMatcherGroup(devtapGroup) {
		t.Error("should detect devtap group")
	}

	otherGroup := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "echo hello"},
		},
	}
	if isDevtapMatcherGroup(otherGroup) {
		t.Error("should not detect non-devtap group")
	}

	if isDevtapMatcherGroup("not a map") {
		t.Error("string should not be detected")
	}
	if isDevtapMatcherGroup(map[string]any{}) {
		t.Error("empty map should not be detected")
	}
}

func TestInstallConfig(t *testing.T) {
	config := adapter.InstallConfig{
		ProjectDir: "/tmp/test",
		AutoLoop:   true,
		MaxRetries: 3,
	}

	if !config.AutoLoop {
		t.Error("expected AutoLoop to be true")
	}
	if config.MaxRetries != 3 {
		t.Error("expected MaxRetries to be 3")
	}
}

func TestInstallMCPOnly(t *testing.T) {
	a := New()
	dir := t.TempDir()

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Should create .mcp.json
	mcpPath := filepath.Join(dir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}

	if !strings.Contains(string(data), "mcp-serve") {
		t.Error(".mcp.json should reference mcp-serve")
	}
}
