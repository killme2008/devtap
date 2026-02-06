package gemini

import "github.com/killme2008/devtap/internal/adapter"

var projectInstructionFiles = []string{"GEMINI.md"}

func installInstruction(projectDir string) {
	adapter.InstallInstruction(projectDir, projectInstructionFiles, adapter.InstructionBlockMCP)
}
