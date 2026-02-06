package capture

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

// Runner captures subprocess output and writes it to a store.
type Runner struct {
	store     store.Store
	sessionID string
	tag       string
	filter    LineFilter
}

// LineFilter decides whether a line should be forwarded.
type LineFilter func(line string) bool

// New creates a new Runner.
func New(s store.Store, sessionID, tag string, filter LineFilter) *Runner {
	return &Runner{
		store:     s,
		sessionID: sessionID,
		tag:       tag,
		filter:    filter,
	}
}

// RunResult holds the result of a subprocess execution.
type RunResult struct {
	ExitCode int
}

// Run executes the given command, captures stdout/stderr, passes them through
// to the terminal transparently, and writes captured output to the store.
func (r *Runner) Run(args []string) (*RunResult, error) {
	if len(args) == 0 {
		return nil, errNoCommand
	}

	cmd := exec.Command(args[0], args[1:]...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		r.captureStream(stdoutPipe, os.Stdout, "stdout")
	}()
	go func() {
		defer wg.Done()
		r.captureStream(stderrPipe, os.Stderr, "stderr")
	}()

	wg.Wait()

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	// Write exit code message
	ec := exitCode
	warnStoreWrite(r.store.Write(r.sessionID, store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       r.tag,
		Stream:    "exit",
		ExitCode:  &ec,
	}))

	return &RunResult{ExitCode: exitCode}, nil
}

func (r *Runner) captureStream(pipe io.ReadCloser, passthrough *os.File, stream string) {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, ScannerInitBuf), ScannerMaxBuf)

	var batch []string

	for scanner.Scan() {
		line := scanner.Text()

		// Always passthrough to terminal
		_, _ = passthrough.WriteString(line + "\n")

		// Apply filter
		if r.filter != nil && !r.filter(line) {
			continue
		}

		batch = append(batch, line)

		// Flush batch periodically to avoid too many small writes
		if len(batch) >= 50 {
			r.flushBatch(batch, stream)
			batch = batch[:0]
		}
	}

	// Flush remaining
	if len(batch) > 0 {
		r.flushBatch(batch, stream)
	}
}

func (r *Runner) flushBatch(lines []string, stream string) {
	linesCopy := make([]string, len(lines))
	copy(linesCopy, lines)

	warnStoreWrite(r.store.Write(r.sessionID, store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       r.tag,
		Stream:    stream,
		Lines:     linesCopy,
	}))
}
