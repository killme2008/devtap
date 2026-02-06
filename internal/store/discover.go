package store

import (
	"os"
	"path/filepath"
)

// DiscoverAdapters returns all registered adapter names for a session.
// It scans baseDir/<sessionID>/ for subdirectories and always includes selfAdapter.
func DiscoverAdapters(baseDir, sessionID, selfAdapter string) []string {
	sessionPath := filepath.Join(baseDir, sessionID)
	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return []string{selfAdapter}
	}

	seen := make(map[string]bool)
	seen[selfAdapter] = true
	for _, e := range entries {
		if e.IsDir() {
			seen[e.Name()] = true
		}
	}

	result := make([]string, 0, len(seen))
	for a := range seen {
		result = append(result, a)
	}
	return result
}

// EnsureAdapterDir creates the adapter directory for a given session,
// registering the adapter so writers can discover it via DiscoverAdapters.
func EnsureAdapterDir(baseDir, sessionID, adapter string) error {
	return os.MkdirAll(filepath.Join(baseDir, sessionID, adapter), 0o755)
}
