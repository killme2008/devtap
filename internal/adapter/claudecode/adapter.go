package claudecode

import (
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
)

// Adapter implements the Claude Code integration via MCP.
type Adapter struct{}

// New creates a new Claude Code adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "claude-code"
}

func (a *Adapter) DiscoverSessions(projectDir string) ([]adapter.Session, error) {
	return discoverSessions(projectDir)
}

// Install writes .mcp.json for MCP server integration,
// optionally configures a Stop hook for auto-loop mode,
// and injects devtap instructions into a project instruction file.
func (a *Adapter) Install(config adapter.InstallConfig) error {
	mcpConfig := filepath.Join(config.ProjectDir, mcpConfigName)
	if adapter.ConfigHasDevtap(mcpConfig) && !adapter.ConfirmOverwrite(mcpConfig) {
		return nil
	}

	if err := writeMCPConfig(config.ProjectDir, config.ExtraArgs); err != nil {
		return err
	}

	if config.AutoLoop {
		if err := installStopHook(config); err != nil {
			return err
		}
	}

	installInstruction(config.ProjectDir)
	return nil
}
