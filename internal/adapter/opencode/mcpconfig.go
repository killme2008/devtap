package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = "opencode.json"

// writeMCPConfig writes or merges devtap MCP server config into opencode.json.
func writeMCPConfig(projectDir string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "devtap"
	}

	configPath := filepath.Join(projectDir, configFileName)

	// Load existing config if present
	existing := make(map[string]any)
	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse existing opencode.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read opencode.json: %w", err)
	}

	// Get or create mcp section
	mcpSection, _ := existing["mcp"].(map[string]any)
	if mcpSection == nil {
		mcpSection = make(map[string]any)
	}

	mcpSection["devtap"] = map[string]any{
		"type":    "local",
		"command": []string{binPath, "mcp-serve"},
	}

	existing["mcp"] = mcpSection

	output, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode.json: %w", err)
	}

	return os.WriteFile(configPath, output, 0o644)
}
