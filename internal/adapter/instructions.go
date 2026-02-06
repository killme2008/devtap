package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const instructionStartMarker = "<!-- devtap:start -->"

// InstructionBlockMCP is the instruction block for MCP-capable adapters.
const InstructionBlockMCP = `<!-- devtap:start -->
## devtap

Get pending build errors and output captured by devtap. Call this before writing or editing code to check for build failures that need fixing.
<!-- devtap:end -->`

// InstructionBlockLint is the instruction block for lint-based adapters (aider).
const InstructionBlockLint = `<!-- devtap:start -->
## devtap

Build errors are automatically checked via the devtap lint script after each edit.
When you see lint output from devtap, fix the reported build errors before continuing.
<!-- devtap:end -->`

// FindProjectInstruction returns the path of the first existing file from paths,
// or an empty string if none exist.
func FindProjectInstruction(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// AppendInstruction appends the devtap instruction block to filePath if the
// marker is not already present. Returns true if the file was modified.
func AppendInstruction(filePath, block string) (bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", filePath, err)
	}

	content := string(data)
	if strings.Contains(content, instructionStartMarker) {
		return false, nil
	}

	// Trim trailing whitespace to avoid excess blank lines, then add
	// a clean double-newline separator before the block.
	trimmed := strings.TrimRight(content, "\n\r\t ")
	sep := "\n\n"
	if len(trimmed) == 0 {
		sep = ""
	}

	newContent := trimmed + sep + block + "\n"
	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", filePath, err)
	}
	return true, nil
}

// InstallInstruction handles the full instruction injection flow:
//   - Find the first existing project instruction file → append (idempotent)
//   - If not found → create the highest priority project file
func InstallInstruction(projectDir string, projectFiles []string, block string) {
	paths := make([]string, len(projectFiles))
	for i, f := range projectFiles {
		paths[i] = filepath.Join(projectDir, f)
	}

	found := FindProjectInstruction(paths)
	if found == "" {
		// Create the highest priority project file.
		found = paths[0]
		if err := os.MkdirAll(filepath.Dir(found), 0o755); err != nil {
			fmt.Printf("Warning: failed to create directory for %s: %v\n", found, err)
			return
		}
		if err := os.WriteFile(found, []byte(block+"\n"), 0o644); err != nil {
			fmt.Printf("Warning: failed to create %s: %v\n", found, err)
			return
		}
		fmt.Printf("Created %s with devtap instructions\n", found)
		return
	}

	modified, err := AppendInstruction(found, block)
	if err != nil {
		fmt.Printf("Warning: failed to update %s: %v\n", found, err)
		return
	}
	if modified {
		fmt.Printf("Added devtap instructions to %s\n", found)
	}
}
