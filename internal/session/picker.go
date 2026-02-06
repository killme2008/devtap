package session

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/killme2008/devtap/internal/adapter"
	"golang.org/x/term"
)

// EncodeDir converts a directory path to a session-safe ID by replacing
// path separators with dashes.
func EncodeDir(dir string) string {
	return strings.ReplaceAll(dir, string(os.PathSeparator), "-")
}

// Pick presents an interactive session picker and returns the chosen session ID.
// Falls back to a simple numbered list if the terminal doesn't support raw mode.
func Pick(sessions []adapter.Session) (string, error) {
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found")
	}
	if len(sessions) == 1 {
		return sessions[0].ID, nil
	}

	// Try interactive picker if terminal supports it
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return interactivePick(sessions)
	}

	return fallbackPick(sessions)
}

func interactivePick(sessions []adapter.Session) (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fallbackPick(sessions)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	selected := 0
	buf := make([]byte, 3)

	for {
		// Clear and render
		fmt.Fprint(os.Stderr, "\r\033[J") // clear from cursor to end
		fmt.Fprintf(os.Stderr, "Select session (↑/↓ to move, Enter to select):\r\n")

		for i, s := range sessions {
			if i == selected {
				fmt.Fprintf(os.Stderr, "  \033[7m> %s\033[0m\r\n", s.Label)
			} else {
				fmt.Fprintf(os.Stderr, "    %s\r\n", s.Label)
			}
		}

		// Move cursor back up
		fmt.Fprintf(os.Stderr, "\033[%dA", len(sessions)+1)

		// Read input
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return fallbackPick(sessions)
		}

		switch {
		case n == 1 && buf[0] == 13: // Enter
			// Clear the picker display
			fmt.Fprint(os.Stderr, "\r\033[J")
			fmt.Fprintf(os.Stderr, "Selected: %s\r\n", sessions[selected].Label)
			return sessions[selected].ID, nil

		case n == 1 && buf[0] == 3: // Ctrl-C
			fmt.Fprint(os.Stderr, "\r\033[J")
			return "", fmt.Errorf("cancelled")

		case n == 1 && buf[0] == 'q': // q to quit
			fmt.Fprint(os.Stderr, "\r\033[J")
			return "", fmt.Errorf("cancelled")

		case n == 3 && buf[0] == 27 && buf[1] == 91: // Arrow keys
			switch buf[2] {
			case 65: // Up
				if selected > 0 {
					selected--
				}
			case 66: // Down
				if selected < len(sessions)-1 {
					selected++
				}
			}

		case n == 1 && buf[0] >= '1' && buf[0] <= '9': // Number keys
			idx := int(buf[0]-'0') - 1
			if idx < len(sessions) {
				fmt.Fprint(os.Stderr, "\r\033[J")
				fmt.Fprintf(os.Stderr, "Selected: %s\r\n", sessions[idx].Label)
				return sessions[idx].ID, nil
			}
		}
	}
}

func fallbackPick(sessions []adapter.Session) (string, error) {
	fmt.Fprintln(os.Stderr, "Available sessions:")
	for i, s := range sessions {
		fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, s.Label)
	}
	fmt.Fprint(os.Stderr, "Select session (1): ")

	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)

	if input == "" {
		return sessions[0].ID, nil
	}

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(sessions) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}
	return sessions[idx-1].ID, nil
}
