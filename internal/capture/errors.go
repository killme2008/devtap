package capture

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var errNoCommand = errors.New("no command specified")

// buildCommand creates an exec.Cmd from args, extracting leading KEY=VALUE
// pairs as environment variables. This supports the common shell pattern:
//
//	devtap -- VITE_LANG=zh pnpm dev
func buildCommand(args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, errNoCommand
	}

	// Split leading KEY=VALUE env vars from the actual command.
	var envVars []string
	i := 0
	for i < len(args) {
		if k, _, ok := strings.Cut(args[i], "="); ok && k != "" && !strings.ContainsAny(k, " \t/\\") {
			envVars = append(envVars, args[i])
			i++
		} else {
			break
		}
	}

	if i >= len(args) {
		return nil, errNoCommand
	}

	cmd := exec.Command(args[i], args[i+1:]...)
	if len(envVars) > 0 {
		cmd.Env = append(os.Environ(), envVars...)
	}
	return cmd, nil
}

// CommandName returns the actual command name from args, skipping leading
// KEY=VALUE environment variable assignments.
func CommandName(args []string) string {
	for _, arg := range args {
		if k, _, ok := strings.Cut(arg, "="); ok && k != "" && !strings.ContainsAny(k, " \t/\\") {
			continue
		}
		return arg
	}
	return ""
}

// Scanner buffer sizes for reading subprocess output.
const (
	ScannerInitBuf = 64 * 1024   // 64 KB initial buffer
	ScannerMaxBuf  = 1024 * 1024 // 1 MB max buffer
)

// warnStoreWrite logs a store write error to stderr without failing the capture.
func warnStoreWrite(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "devtap: store write: %v\n", err)
	}
}
