package filter

import "fmt"

// Truncate applies smart truncation to a list of lines:
// - If lines exceed maxLines, keeps head and tail with an omission notice in between.
// - Merges consecutive duplicate lines into "(repeated N times)" markers.
// maxLines <= 0 means no truncation.
func Truncate(lines []string, maxLines int) []string {
	lines = dedup(lines)

	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}

	// Keep roughly half at the head and half at the tail
	head := maxLines / 2
	tail := maxLines - head
	if tail == 0 {
		tail = 1
		head = maxLines - 1
	}

	omitted := len(lines) - head - tail
	result := make([]string, 0, head+1+tail)
	result = append(result, lines[:head]...)
	result = append(result, fmt.Sprintf("... (%d lines omitted)", omitted))
	result = append(result, lines[len(lines)-tail:]...)

	return result
}

// dedup merges consecutive duplicate lines into "(repeated N times)" markers.
func dedup(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	result := make([]string, 0, len(lines))
	prev := lines[0]
	count := 1

	for i := 1; i < len(lines); i++ {
		if lines[i] == prev {
			count++
		} else {
			result = append(result, prev)
			if count > 1 {
				result = append(result, fmt.Sprintf("(repeated %d times)", count))
			}
			prev = lines[i]
			count = 1
		}
	}

	result = append(result, prev)
	if count > 1 {
		result = append(result, fmt.Sprintf("(repeated %d times)", count))
	}

	return result
}
