package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindProjectInstruction(t *testing.T) {
	dir := t.TempDir()

	t.Run("returns first existing file", func(t *testing.T) {
		first := filepath.Join(dir, "FIRST.md")
		second := filepath.Join(dir, "SECOND.md")
		_ = os.WriteFile(first, []byte("# first"), 0o644)
		_ = os.WriteFile(second, []byte("# second"), 0o644)

		got := FindProjectInstruction([]string{first, second})
		if got != first {
			t.Errorf("got %q, want %q", got, first)
		}
	})

	t.Run("skips missing files", func(t *testing.T) {
		existing := filepath.Join(dir, "EXISTS.md")
		_ = os.WriteFile(existing, []byte("# exists"), 0o644)

		got := FindProjectInstruction([]string{filepath.Join(dir, "MISSING.md"), existing})
		if got != existing {
			t.Errorf("got %q, want %q", got, existing)
		}
	})

	t.Run("returns empty when none exist", func(t *testing.T) {
		got := FindProjectInstruction([]string{filepath.Join(dir, "A.md"), filepath.Join(dir, "B.md")})
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestAppendInstruction(t *testing.T) {
	block := InstructionBlockMCP

	t.Run("appends to file with existing content", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "CLAUDE.md")
		_ = os.WriteFile(f, []byte("# My Project"), 0o644)

		modified, err := AppendInstruction(f, block)
		if err != nil {
			t.Fatal(err)
		}
		if !modified {
			t.Error("expected modified=true")
		}

		data, _ := os.ReadFile(f)
		content := string(data)

		if !strings.HasPrefix(content, "# My Project") {
			t.Error("original content not preserved")
		}
		if content[12:14] != "\n\n" {
			t.Errorf("expected separator, got %q", content[12:14])
		}
		if !strings.Contains(content, instructionStartMarker) {
			t.Error("marker not found in content")
		}
		if content[len(content)-1] != '\n' {
			t.Error("expected trailing newline")
		}
	})

	t.Run("appends to empty file without extra newlines", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "CLAUDE.md")
		_ = os.WriteFile(f, []byte(""), 0o644)

		modified, err := AppendInstruction(f, block)
		if err != nil {
			t.Fatal(err)
		}
		if !modified {
			t.Error("expected modified=true")
		}

		data, _ := os.ReadFile(f)
		content := string(data)

		if !strings.HasPrefix(content, instructionStartMarker) {
			t.Errorf("expected block at start, got %q", content[:20])
		}
	})

	t.Run("skips when marker already present", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "CLAUDE.md")
		original := "# Project\n\n" + block + "\n"
		_ = os.WriteFile(f, []byte(original), 0o644)

		modified, err := AppendInstruction(f, block)
		if err != nil {
			t.Fatal(err)
		}
		if modified {
			t.Error("expected modified=false when marker exists")
		}

		data, _ := os.ReadFile(f)
		if string(data) != original {
			t.Error("content was modified")
		}
	})

	t.Run("trims trailing newlines before appending", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "CLAUDE.md")
		_ = os.WriteFile(f, []byte("# My Project\n\n\n"), 0o644)

		modified, err := AppendInstruction(f, block)
		if err != nil {
			t.Fatal(err)
		}
		if !modified {
			t.Error("expected modified=true")
		}

		data, _ := os.ReadFile(f)
		content := string(data)

		want := "# My Project\n\n" + block + "\n"
		if content != want {
			t.Errorf("got:\n%s\nwant:\n%s", content, want)
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := AppendInstruction(filepath.Join(t.TempDir(), "MISSING.md"), block)
		if err == nil {
			t.Error("expected error for missing file")
		}
	})
}

func TestConfigHasDevtap(t *testing.T) {
	t.Run("true when file contains devtap", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), ".mcp.json")
		_ = os.WriteFile(f, []byte(`{"mcpServers":{"devtap":{}}}`), 0o644)

		if !ConfigHasDevtap(f) {
			t.Error("expected true")
		}
	})

	t.Run("false when file has no devtap", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), ".mcp.json")
		_ = os.WriteFile(f, []byte(`{"mcpServers":{"other":{}}}`), 0o644)

		if ConfigHasDevtap(f) {
			t.Error("expected false")
		}
	})

	t.Run("false when file does not exist", func(t *testing.T) {
		if ConfigHasDevtap(filepath.Join(t.TempDir(), "missing.json")) {
			t.Error("expected false for missing file")
		}
	})
}
