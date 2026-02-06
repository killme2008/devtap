package claudecode

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/killme2008/devtap/internal/adapter"
)

// claudeProjectsDir returns the path to Claude Code's projects directory.
func claudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// encodeProjectDir encodes a project directory path to Claude Code's format.
// e.g., "/Users/dennis/foo" -> "-Users-dennis-foo"
func encodeProjectDir(dir string) string {
	return strings.ReplaceAll(dir, string(os.PathSeparator), "-")
}

// discoverSessions finds active Claude Code sessions for the given project dir.
func discoverSessions(projectDir string) ([]adapter.Session, error) {
	projDir, err := claudeProjectsDir()
	if err != nil {
		return nil, err
	}

	encoded := encodeProjectDir(projectDir)
	sessionDir := filepath.Join(projDir, encoded)

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	type sessionEntry struct {
		session adapter.Session
		modTime int64
	}

	var sessions []sessionEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		sessions = append(sessions, sessionEntry{
			session: adapter.Session{
				ID:         sessionID,
				ProjectDir: projectDir,
				Label:      fmt.Sprintf("Claude Code session %s (modified %s)", sessionID[:8], info.ModTime().Format("15:04:05")),
			},
			modTime: info.ModTime().Unix(),
		})
	}

	// Sort by most recently modified first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].modTime > sessions[j].modTime
	})

	result := make([]adapter.Session, len(sessions))
	for i, s := range sessions {
		result[i] = s.session
	}
	return result, nil
}
