package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectDir_GitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	subdir := filepath.Join(root, "frontend", "app")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	got := resolveProjectDir(subdir)
	if got != root {
		t.Fatalf("expected git root %q, got %q", root, got)
	}
}

func TestResolveProjectDir_MarkerFallback(t *testing.T) {
	root := t.TempDir()
	if err := touchFile(filepath.Join(root, "package.json")); err != nil {
		t.Fatalf("touch package.json: %v", err)
	}

	subdir := filepath.Join(root, "frontend")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	got := resolveProjectDir(subdir)
	if got != root {
		t.Fatalf("expected marker root %q, got %q", root, got)
	}
}

func TestResolveProjectDir_FallbackToStart(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "frontend")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	got := resolveProjectDir(subdir)
	if got != subdir {
		t.Fatalf("expected start dir %q, got %q", subdir, got)
	}
}

func TestResolveProjectDir_GitPriorityOverMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir appDir: %v", err)
	}
	if err := touchFile(filepath.Join(appDir, "go.mod")); err != nil {
		t.Fatalf("touch go.mod: %v", err)
	}

	subdir := filepath.Join(appDir, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	got := resolveProjectDir(subdir)
	if got != root {
		t.Fatalf("expected git root %q, got %q", root, got)
	}
}

func TestShellJoin(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple", []string{"echo", "hello"}, `echo hello`},
		{"space in arg", []string{"echo", "hello world"}, `echo "hello world"`},
		{"multiple spaces", []string{"grep", "-r", "foo bar", "src dir"}, `grep -r "foo bar" "src dir"`},
		{"quotes in arg", []string{"echo", `say "hi"`}, `echo "say \"hi\""`},
		{"shell metachar", []string{"bash", "-c", "echo $HOME"}, `bash -c "echo $HOME"`},
		{"backslash", []string{"echo", `a\nb`}, `echo "a\\nb"`},
		{"no args needing quotes", []string{"cargo", "build", "--release"}, `cargo build --release`},
		{"single arg", []string{"make"}, `make`},
		{"pipe char", []string{"sh", "-c", "cat | grep foo"}, `sh -c "cat | grep foo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellJoin(tt.args)
			if got != tt.want {
				t.Errorf("shellJoin(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func touchFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
