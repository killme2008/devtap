package adapter

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfigHasDevtap checks whether the config file at path exists and already
// contains a devtap configuration entry.
func ConfigHasDevtap(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "devtap")
}

// ConfirmOverwrite prompts the user to confirm overwriting an existing
// devtap configuration. Returns true if the user answers yes.
func ConfirmOverwrite(path string) bool {
	fmt.Printf("devtap is already configured in %s. Overwrite? [y/N] ", path)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}
