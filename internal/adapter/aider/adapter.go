package aider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
	"github.com/killme2008/devtap/internal/session"
)

const lintScriptName = ".devtap-aider-lint.sh"

// Adapter implements the aider integration.
//
// aider has no MCP support. Integration strategy:
//   - Install creates a lint wrapper script for use with --lint-cmd.
//     The script calls devtap drain and outputs errors as plain text.
//   - Usage: aider --lint-cmd "sh .devtap-aider-lint.sh" --auto-lint
type Adapter struct{}

// New creates a new aider adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "aider"
}

// DiscoverSessions returns sessions based on the project directory.
// aider doesn't have persistent sessions.
func (a *Adapter) DiscoverSessions(projectDir string) ([]adapter.Session, error) {
	return []adapter.Session{
		{
			ID:         session.EncodeDir(projectDir),
			ProjectDir: projectDir,
			Label:      fmt.Sprintf("aider project: %s", filepath.Base(projectDir)),
		},
	}, nil
}

// Install creates a lint wrapper script for aider integration
// and injects devtap instructions into a project instruction file.
func (a *Adapter) Install(config adapter.InstallConfig) error {
	scriptPath := filepath.Join(config.ProjectDir, lintScriptName)
	if adapter.ConfigHasDevtap(scriptPath) && !adapter.ConfirmOverwrite(scriptPath) {
		return nil
	}

	if err := createLintScript(config.ProjectDir, config.ExtraArgs); err != nil {
		return err
	}
	installInstruction(config.ProjectDir)
	return nil
}

func createLintScript(projectDir string, extraArgs []string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "devtap"
	}

	drainCmd := fmt.Sprintf("%s drain", binPath)
	for _, arg := range extraArgs {
		drainCmd += fmt.Sprintf(" %s", arg)
	}

	// The lint script drains pending devtap messages and outputs plain text.
	// aider --lint-cmd expects: outputs errors to stdout, non-zero exit if errors.
	script := fmt.Sprintf(`#!/bin/sh
# devtap lint wrapper for aider
# Usage: aider --lint-cmd "sh %s" --auto-lint
#
# This script is called by aider after each edit.
# It drains pending build errors from devtap and presents them to aider.

OUTPUT=$(%s 2>/dev/null)

if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    exit 1
fi

exit 0
`, lintScriptName, drainCmd)

	scriptPath := filepath.Join(projectDir, lintScriptName)
	return os.WriteFile(scriptPath, []byte(script), 0o755)
}
