package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const instructionStartMarker = "<!-- devtap:start -->"
const instructionEndMarker = "<!-- devtap:end -->"

// InstructionBlockMCP is the instruction block for MCP-capable adapters.
const InstructionBlockMCP = `<!-- devtap:start -->
## devtap

devtap captures build/dev output from a separate terminal and delivers it here via MCP.

**You MUST check devtap in these situations:**
- At the start of each new task or conversation turn
- Before writing or editing any code
- After being told that a build, test, or dev server has new output

**Use this call order:**
1. Call "get_build_status" once at the start of the turn.
2. Call "get_build_errors" when status reports pending messages, when the user says new output arrived, or before the first code edit in the turn.

**Call limit per turn:** call "get_build_errors" at most once per turn unless new output is reported.

**Multi-source mode:** when devtap drains from multiple sources, tags are prefixed with "host/label |" (for example, "[devtap: myhost/local | make]"). "host" is the machine name, "label" identifies the source. Show these prefixes as-is. If output includes source warnings (for example, source unreachable), show those warnings verbatim and continue with output from reachable sources.

**Output format:** when "get_build_errors" returns content:
If build succeeded, acknowledge briefly (do not repeat the output).
If build failed, present the error output verbatim in a fenced code block.
Then add one line: "Next action: <what you will do>".
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

// AppendInstruction upserts the devtap instruction block in filePath.
// If a devtap block exists, it is replaced with block; otherwise block is appended.
// Returns true if the file content changed.
func AppendInstruction(filePath, block string) (bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", filePath, err)
	}

	content := string(data)
	if start := strings.Index(content, instructionStartMarker); start >= 0 {
		endRel := strings.Index(content[start:], instructionEndMarker)
		if endRel < 0 {
			return false, fmt.Errorf("updating %s: found %q but missing %q", filePath, instructionStartMarker, instructionEndMarker)
		}

		end := start + endRel + len(instructionEndMarker)
		newContent := content[:start] + block + content[end:]
		if !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		if newContent == content {
			return false, nil
		}
		if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
			return false, fmt.Errorf("writing %s: %w", filePath, err)
		}
		return true, nil
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
