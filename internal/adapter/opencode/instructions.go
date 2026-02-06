package opencode

import "github.com/killme2008/devtap/internal/adapter"

var projectInstructionFiles = []string{"AGENTS.md", "CLAUDE.md"}

func installInstruction(projectDir string) {
	adapter.InstallInstruction(projectDir, projectInstructionFiles, adapter.InstructionBlockMCP)
}
