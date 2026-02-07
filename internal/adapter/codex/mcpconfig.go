package codex

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// codexConfig represents the relevant portion of .codex/config.toml.
type codexConfig struct {
	MCPServers map[string]mcpServerEntry `toml:"mcp_servers"`
}

type mcpServerEntry struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args,omitempty"`
}

const mcpConfigPath = ".codex/config.toml"

// writeMCPConfig writes or merges devtap MCP server config into .codex/config.toml.
func writeMCPConfig(projectDir string, extraArgs []string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "devtap"
	}

	configPath := filepath.Join(projectDir, mcpConfigPath)
	configDir := filepath.Dir(configPath)

	// Load existing config if present
	var cfg codexConfig
	data, err := os.ReadFile(configPath)
	if err == nil {
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return fmt.Errorf("parse existing .codex/config.toml: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read .codex/config.toml: %w", err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]mcpServerEntry)
	}

	cfg.MCPServers["devtap"] = mcpServerEntry{
		Command: binPath,
		Args:    append([]string{"mcp-serve"}, extraArgs...),
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create .codex dir: %w", err)
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	return os.WriteFile(configPath, buf.Bytes(), 0o644)
}
