package aider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/killme2008/devtap/internal/adapter"
)

func TestInstall(t *testing.T) {
	a := New()
	dir := t.TempDir()

	config := adapter.InstallConfig{ProjectDir: dir}
	if err := a.Install(config); err != nil {
		t.Fatalf("Install: %v", err)
	}

	scriptPath := filepath.Join(dir, lintScriptName)
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("lint script not created: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Error("lint script should be executable")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if !strings.Contains(string(content), "drain") {
		t.Error("script should call drain command")
	}
}

func TestDiscoverSessions(t *testing.T) {
	a := New()
	sessions, err := a.DiscoverSessions("/tmp/test-project")
	if err != nil {
		t.Fatalf("DiscoverSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}
