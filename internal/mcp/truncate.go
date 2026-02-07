package mcp

import (
	"github.com/killme2008/devtap/internal/filter"
	"github.com/killme2008/devtap/internal/store"
)

// TruncateMessages applies line-level truncation across messages.
// It allocates the maxLines budget proportionally to each message,
// using filter.Truncate for tail-biased truncation per message.
func TruncateMessages(messages []store.LogMessage, maxLines int) []store.LogMessage {
	if maxLines <= 0 {
		return messages
	}

	var totalLines int
	for _, msg := range messages {
		totalLines += len(msg.Lines)
	}

	if totalLines <= maxLines {
		return messages
	}

	// Allocate line budget proportionally to each message.
	result := make([]store.LogMessage, len(messages))
	copy(result, messages)

	remaining := maxLines
	for i := range result {
		if len(result[i].Lines) == 0 {
			continue
		}
		// Proportional share, minimum 1 line per non-empty message.
		share := len(result[i].Lines) * maxLines / totalLines
		if share < 1 {
			share = 1
		}
		if share > remaining {
			share = remaining
		}
		result[i].Lines = filter.Truncate(result[i].Lines, share)
		remaining -= len(result[i].Lines)
		if remaining <= 0 {
			// Clear remaining messages' lines.
			for j := i + 1; j < len(result); j++ {
				result[j].Lines = nil
			}
			break
		}
	}

	return result
}
