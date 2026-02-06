package capture

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

// LongRunner captures output from a long-running subprocess with debounce.
// Unlike Runner which flushes in batches of N lines, LongRunner collects
// output and flushes at regular intervals (debounce duration).
type LongRunner struct {
	store     store.Store
	sessionID string
	tag       string
	filter    LineFilter
	debounce  time.Duration
}

// NewLongRunner creates a new LongRunner with the specified debounce duration.
func NewLongRunner(s store.Store, sessionID, tag string, filter LineFilter, debounce time.Duration) *LongRunner {
	return &LongRunner{
		store:     s,
		sessionID: sessionID,
		tag:       tag,
		filter:    filter,
		debounce:  debounce,
	}
}

// Run executes the command and captures output with debounced flushing.
// It forwards signals (SIGINT, SIGTERM) to the child process.
func (r *LongRunner) Run(args []string) (*RunResult, error) {
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

	// Forward signals to child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()
	defer signal.Stop(sigCh)

	// Debounce buffers
	var mu sync.Mutex
	stdoutBuf := make([]string, 0, 64)
	stderrBuf := make([]string, 0, 64)

	// Periodic flush ticker
	ticker := time.NewTicker(r.debounce)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				r.flushBuffer(&stdoutBuf, "stdout")
				r.flushBuffer(&stderrBuf, "stderr")
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		r.captureStreamDebounced(stdoutPipe, os.Stdout, &mu, &stdoutBuf)
	}()
	go func() {
		defer wg.Done()
		r.captureStreamDebounced(stderrPipe, os.Stderr, &mu, &stderrBuf)
	}()

	wg.Wait()
	close(done)

	// Final flush
	mu.Lock()
	r.flushBuffer(&stdoutBuf, "stdout")
	r.flushBuffer(&stderrBuf, "stderr")
	mu.Unlock()

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	// Write exit code
	ec := exitCode
	warnStoreWrite(r.store.Write(r.sessionID, store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       r.tag,
		Stream:    "exit",
		ExitCode:  &ec,
	}))

	return &RunResult{ExitCode: exitCode}, nil
}

func (r *LongRunner) captureStreamDebounced(pipe io.ReadCloser, passthrough *os.File, mu *sync.Mutex, buf *[]string) {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, ScannerInitBuf), ScannerMaxBuf)

	for scanner.Scan() {
		line := scanner.Text()

		// Always passthrough
		_, _ = passthrough.WriteString(line + "\n")

		// Apply filter
		if r.filter != nil && !r.filter(line) {
			continue
		}

		mu.Lock()
		*buf = append(*buf, line)
		mu.Unlock()
	}
}

func (r *LongRunner) flushBuffer(buf *[]string, stream string) {
	if len(*buf) == 0 {
		return
	}

	lines := make([]string, len(*buf))
	copy(lines, *buf)
	*buf = (*buf)[:0]

	warnStoreWrite(r.store.Write(r.sessionID, store.LogMessage{
		Timestamp: time.Now().UTC(),
		Tag:       r.tag,
		Stream:    stream,
		Lines:     lines,
	}))
}
