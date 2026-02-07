package opencode

import (
	"fmt"
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
	"github.com/killme2008/devtap/internal/session"
)

// Adapter implements the OpenCode integration via MCP.
type Adapter struct{}

// New creates a new OpenCode adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "opencode"
}

// DiscoverSessions returns sessions based on the project directory.
// OpenCode doesn't expose persistent session IDs externally.
func (a *Adapter) DiscoverSessions(projectDir string) ([]adapter.Session, error) {
	return []adapter.Session{
		{
			ID:         session.EncodeDir(projectDir),
			ProjectDir: projectDir,
			Label:      fmt.Sprintf("OpenCode project: %s", filepath.Base(projectDir)),
		},
	}, nil
}

// Install writes opencode.json with devtap MCP server configuration
// and injects devtap instructions into a project instruction file.
func (a *Adapter) Install(config adapter.InstallConfig) error {
	mcpConfig := filepath.Join(config.ProjectDir, configFileName)
	if adapter.ConfigHasDevtap(mcpConfig) && !adapter.ConfirmOverwrite(mcpConfig) {
		return nil
	}

	if err := writeMCPConfig(config.ProjectDir, config.ExtraArgs); err != nil {
		return err
	}
	installInstruction(config.ProjectDir)
	return nil
}
