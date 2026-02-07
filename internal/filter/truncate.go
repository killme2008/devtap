package filter

import "fmt"

// Truncate applies smart truncation with a 50/50 head/tail split.
// See TruncateWithRatio for details.
func Truncate(lines []string, maxLines int) []string {
	return TruncateWithRatio(lines, maxLines, 0.5)
}

// TruncateWithRatio applies smart truncation to a list of lines:
//   - Merges consecutive duplicate lines into "(repeated N times)" markers.
//   - If lines exceed maxLines, keeps head and tail with an omission notice in between.
//   - tailRatio controls what fraction of the budget goes to the tail (0.0–1.0).
//     For build output where errors appear at the end, use tailRatio=0.8.
//
// maxLines <= 0 means no truncation.
func TruncateWithRatio(lines []string, maxLines int, tailRatio float64) []string {
	lines = dedup(lines)

	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}

	// Budget=1: no room for head + omission marker + tail. Just keep the last line.
	if maxLines == 1 {
		return lines[len(lines)-1:]
	}

	tail := int(float64(maxLines) * tailRatio)
	if tail > maxLines {
		tail = maxLines
	}
	head := maxLines - tail
	// Ensure at least 1 line on each side when budget allows.
	if tail == 0 && maxLines > 1 {
		tail = 1
		head = maxLines - 1
	}
	if head == 0 && maxLines > 1 {
		head = 1
		tail = maxLines - 1
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
