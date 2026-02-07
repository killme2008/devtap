package mcp

import (
	"fmt"

	"github.com/killme2008/devtap/internal/store"
)

// CollapseSuccessful replaces the output of successful build runs (exit code 0)
// with a single-line summary to reduce context token consumption.
//
// A "run" is a sequence of messages sharing the same tag, bounded by an exit
// code message. When the same tag appears in multiple runs within a single
// drain window (e.g. a failed build followed by a successful rebuild), each
// run is evaluated independently — only runs with exit code 0 are collapsed.
// Runs with non-zero exit code or no exit code are returned unchanged.
func CollapseSuccessful(messages []store.LogMessage) []store.LogMessage {
	type run struct {
		firstIdx int
		indices  []int
		exitCode *int
		lines    int
	}

	// Work on a copy so we can replace run heads in-place while preserving the
	// original global message order.
	result := make([]store.LogMessage, len(messages))
	copy(result, messages)
	removed := make([]bool, len(result))

	// current tracks the active (not yet finalized) run per tag.
	current := make(map[string]*run)

	for i, msg := range result {
		tag := msg.Tag
		if tag == "" {
			tag = "build"
		}

		r, exists := current[tag]
		if !exists {
			r = &run{firstIdx: i}
			current[tag] = r
		}

		r.indices = append(r.indices, i)
		r.lines += len(msg.Lines)
		if msg.ExitCode != nil {
			r.exitCode = msg.ExitCode
			if *r.exitCode == 0 && r.lines > 0 {
				// Collapse successful run into a single summary message placed at the
				// first message position to preserve global ordering.
				first := result[r.firstIdx]
				result[r.firstIdx] = store.LogMessage{
					Timestamp: first.Timestamp,
					Tag:       first.Tag,
					Stream:    first.Stream,
					ExitCode:  r.exitCode,
					Adapter:   first.Adapter,
					Host:      first.Host,
					Lines:     []string{collapseMessage(r.lines)},
				}
				for _, idx := range r.indices[1:] {
					removed[idx] = true
				}
			}
			// Finalize this run; next message for the same tag starts a new run.
			delete(current, tag)
		}
	}

	final := make([]store.LogMessage, 0, len(result))
	for i, msg := range result {
		if !removed[i] {
			final = append(final, msg)
		}
	}

	return final
}

func collapseMessage(lineCount int) string {
	noun := "lines"
	if lineCount == 1 {
		noun = "line"
	}
	return fmt.Sprintf("(%d %s of output omitted — build succeeded)", lineCount, noun)
}
