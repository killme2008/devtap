package store

import "time"

// LogMessage represents a captured log entry from a subprocess.
type LogMessage struct {
	Timestamp time.Time `json:"ts"`
	Tag       string    `json:"tag"`
	Stream    string    `json:"stream"` // "stdout" | "stderr"
	Lines     []string  `json:"lines"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	Adapter   string    `json:"adapter,omitempty"`
}

// Store abstracts the log storage and retrieval mechanism.
type Store interface {
	// Write appends a log message to the pending queue for a session.
	Write(sessionID string, msg LogMessage) error

	// Drain reads and consumes all pending messages for a session since last drain.
	Drain(sessionID string, maxLines int) ([]LogMessage, error)

	// Status returns pending message counts per session.
	Status() (map[string]int, error)

	// Close releases any resources held by the store.
	Close() error
}
