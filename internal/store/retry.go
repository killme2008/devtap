package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const retryCountFile = "retry_count"

// RetryTracker manages auto-loop retry counts per session.
type RetryTracker struct {
	baseDir string
}

// NewRetryTracker creates a new retry tracker rooted at baseDir.
func NewRetryTracker(baseDir string) *RetryTracker {
	return &RetryTracker{baseDir: baseDir}
}

func (r *RetryTracker) retryPath(sessionID string) string {
	return filepath.Join(r.baseDir, sessionID, retryCountFile)
}

// Get returns the current retry count for a session.
func (r *RetryTracker) Get(sessionID string) (int, error) {
	data, err := os.ReadFile(r.retryPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read retry count: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, nil // corrupted file, treat as 0
	}
	return count, nil
}

// Increment increments and returns the new retry count.
func (r *RetryTracker) Increment(sessionID string) (int, error) {
	current, err := r.Get(sessionID)
	if err != nil {
		return 0, err
	}
	next := current + 1

	dir := filepath.Dir(r.retryPath(sessionID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create session dir: %w", err)
	}

	if err := os.WriteFile(r.retryPath(sessionID), []byte(strconv.Itoa(next)), 0o644); err != nil {
		return 0, fmt.Errorf("write retry count: %w", err)
	}
	return next, nil
}

// Reset resets the retry count for a session.
func (r *RetryTracker) Reset(sessionID string) error {
	err := os.Remove(r.retryPath(sessionID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset retry count: %w", err)
	}
	return nil
}
