package codex

import (
	"fmt"
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
	"github.com/killme2008/devtap/internal/session"
)

// Adapter implements the Codex CLI integration via MCP.
type Adapter struct{}

// New creates a new Codex CLI adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "codex"
}

// DiscoverSessions returns sessions based on the project directory.
// Codex CLI doesn't have persistent sessions like Claude Code.
func (a *Adapter) DiscoverSessions(projectDir string) ([]adapter.Session, error) {
	return []adapter.Session{
		{
			ID:         session.EncodeDir(projectDir),
			ProjectDir: projectDir,
			Label:      fmt.Sprintf("Codex project: %s", filepath.Base(projectDir)),
		},
	}, nil
}

// Install writes .codex/config.toml for MCP server integration
// and injects devtap instructions into a project instruction file.
func (a *Adapter) Install(config adapter.InstallConfig) error {
	mcpConfig := filepath.Join(config.ProjectDir, mcpConfigPath)
	if adapter.ConfigHasDevtap(mcpConfig) && !adapter.ConfirmOverwrite(mcpConfig) {
		return nil
	}

	if err := writeMCPConfig(config.ProjectDir, config.ExtraArgs); err != nil {
		return err
	}
	installInstruction(config.ProjectDir)
	return nil
}
