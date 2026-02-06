package aider

import "github.com/killme2008/devtap/internal/adapter"

var projectInstructionFiles = []string{"CONVENTIONS.md"}

func installInstruction(projectDir string) {
	adapter.InstallInstruction(projectDir, projectInstructionFiles, adapter.InstructionBlockLint)
}
