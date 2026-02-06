package gemini

import (
	"fmt"
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
	"github.com/killme2008/devtap/internal/session"
)

// Adapter implements the Gemini CLI integration via MCP.
type Adapter struct{}

// New creates a new Gemini CLI adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "gemini"
}

// DiscoverSessions returns sessions based on the project directory.
// Gemini CLI doesn't expose persistent session IDs externally.
func (a *Adapter) DiscoverSessions(projectDir string) ([]adapter.Session, error) {
	return []adapter.Session{
		{
			ID:         session.EncodeDir(projectDir),
			ProjectDir: projectDir,
			Label:      fmt.Sprintf("Gemini CLI project: %s", filepath.Base(projectDir)),
		},
	}, nil
}

// Install writes .gemini/settings.json with devtap MCP server configuration
// and injects devtap instructions into a project instruction file.
func (a *Adapter) Install(config adapter.InstallConfig) error {
	configPath := filepath.Join(config.ProjectDir, configFilePath)
	if adapter.ConfigHasDevtap(configPath) && !adapter.ConfirmOverwrite(configPath) {
		return nil
	}

	if err := writeMCPConfig(config.ProjectDir); err != nil {
		return err
	}
	installInstruction(config.ProjectDir)
	return nil
}
