package claudecode

import (
	"path/filepath"

	"github.com/killme2008/devtap/internal/adapter"
)

var projectInstructionFiles = []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md")}

func installInstruction(projectDir string) {
	adapter.InstallInstruction(projectDir, projectInstructionFiles, adapter.InstructionBlockMCP)
}
