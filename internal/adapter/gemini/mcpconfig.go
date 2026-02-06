package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var configFilePath = filepath.Join(".gemini", "settings.json")

// writeMCPConfig writes or merges devtap MCP server config into .gemini/settings.json.
func writeMCPConfig(projectDir string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "devtap"
	}

	configPath := filepath.Join(projectDir, configFilePath)

	// Load existing config if present
	existing := make(map[string]any)
	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse existing .gemini/settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read .gemini/settings.json: %w", err)
	}

	// Get or create mcpServers section
	servers, _ := existing["mcpServers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}

	servers["devtap"] = map[string]any{
		"command": binPath,
		"args":    []string{"mcp-serve"},
	}

	existing["mcpServers"] = servers

	// Ensure .gemini directory exists
	if err := os.MkdirAll(filepath.Join(projectDir, ".gemini"), 0o755); err != nil {
		return fmt.Errorf("create .gemini dir: %w", err)
	}

	output, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal .gemini/settings.json: %w", err)
	}

	return os.WriteFile(configPath, output, 0o644)
}
