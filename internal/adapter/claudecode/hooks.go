package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/killme2008/devtap/internal/adapter"
)

const mcpConfigName = ".mcp.json"

// writeMCPConfig writes or merges devtap MCP server config into .mcp.json.
func writeMCPConfig(projectDir string, extraArgs []string) error {
	binPath, err := devtapBinaryPath()
	if err != nil {
		return err
	}

	configPath := filepath.Join(projectDir, mcpConfigName)

	// Load existing config if present
	existing := make(map[string]any)
	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse existing .mcp.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read .mcp.json: %w", err)
	}

	// Get or create mcpServers section
	servers, _ := existing["mcpServers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}

	args := append([]string{"mcp-serve"}, extraArgs...)
	servers["devtap"] = map[string]any{
		"command": binPath,
		"args":    args,
	}

	existing["mcpServers"] = servers

	output, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal MCP config: %w", err)
	}

	return os.WriteFile(configPath, output, 0o644)
}

// installStopHook configures only the Stop hook in ~/.claude/settings.json
// for auto-loop mode. This is the only hook needed — MCP handles everything else.
func installStopHook(config adapter.InstallConfig) error {
	settingsFile, err := settingsPath()
	if err != nil {
		return err
	}

	binPath, err := devtapBinaryPath()
	if err != nil {
		return err
	}

	settings := make(map[string]any)
	data, err := os.ReadFile(settingsFile)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read settings.json: %w", err)
	}

	hooks := make(map[string]any)
	if existing, ok := settings["hooks"]; ok {
		if m, ok := existing.(map[string]any); ok {
			hooks = m
		}
	}

	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	stopCmd := fmt.Sprintf("\"%s\" drain --event Stop --auto-loop --max-retries %d", binPath, maxRetries)
	for _, arg := range config.ExtraArgs {
		stopCmd += fmt.Sprintf(" %s", arg)
	}
	hooks["Stop"] = upsertMatcherGroup(hooks["Stop"], "", stopCmd)

	settings["hooks"] = hooks

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	return os.WriteFile(settingsFile, output, 0o644)
}

// settingsPath returns the path to Claude Code's settings.json.
func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// devtapBinaryPath returns the path to the devtap binary.
func devtapBinaryPath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "devtap", nil
	}
	return path, nil
}

// upsertMatcherGroup ensures a devtap hook command exists in the matcher groups
// for an event, replacing any existing devtap entry to avoid duplicates.
func upsertMatcherGroup(existing any, matcher, command string) []any {
	hook := map[string]any{
		"type":    "command",
		"command": command,
	}

	group := map[string]any{
		"hooks": []any{hook},
	}
	if matcher != "" {
		group["matcher"] = matcher
	}

	var groups []any
	if existing != nil {
		if arr, ok := existing.([]any); ok {
			for _, item := range arr {
				if isDevtapMatcherGroup(item) {
					continue
				}
				groups = append(groups, item)
			}
		}
	}
	groups = append(groups, group)
	return groups
}

// isDevtapMatcherGroup checks if a matcher group contains a devtap hook command.
func isDevtapMatcherGroup(item any) bool {
	m, ok := item.(map[string]any)
	if !ok {
		return false
	}
	hooks, ok := m["hooks"]
	if !ok {
		return false
	}
	arr, ok := hooks.([]any)
	if !ok {
		return false
	}
	for _, h := range arr {
		if hm, ok := h.(map[string]any); ok {
			if cmd, ok := hm["command"].(string); ok {
				if strings.Contains(cmd, "devtap") {
					return true
				}
			}
		}
	}
	return false
}
