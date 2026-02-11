package main

import (
	"errors"
	"sync"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

const defaultLazyCooldown = 30 * time.Second

// lazyStore wraps a store.Store that is connected on first use and retried
// with a cooldown on failure. This allows the MCP server to start even when
// the configured store (e.g. GreptimeDB) is temporarily unavailable, and
// reconnect once it becomes reachable.
type lazyStore struct {
	mu          sync.Mutex
	inner       store.Store
	connectFn   func() (store.Store, error)
	lastAttempt time.Time
	cooldown    time.Duration
	closed      bool
}

func newLazyStore(connectFn func() (store.Store, error)) *lazyStore {
	return &lazyStore{
		connectFn: connectFn,
		cooldown:  defaultLazyCooldown,
	}
}

// ensureConnected attempts to establish the inner store if not already connected.
// Returns nil if connected, or the connection error if unavailable.
func (ls *lazyStore) ensureConnected() error {
	if ls.closed {
		return errors.New("store closed")
	}
	if ls.inner != nil {
		return nil
	}
	if !ls.lastAttempt.IsZero() && time.Since(ls.lastAttempt) < ls.cooldown {
		return errors.New("store unavailable (cooldown)")
	}
	ls.lastAttempt = time.Now()
	s, err := ls.connectFn()
	if err != nil {
		return err
	}
	ls.inner = s
	return nil
}

// reset closes the inner store and nils it out so the next call retries.
// Does NOT update lastAttempt — the next ensureConnected checks elapsed time
// since the original connection, which is typically >cooldown, allowing an
// immediate retry after detecting failure.
func (ls *lazyStore) reset() {
	if ls.inner != nil {
		_ = ls.inner.Close()
		ls.inner = nil
	}
}

func (ls *lazyStore) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if err := ls.ensureConnected(); err != nil {
		return nil, err
	}
	msgs, err := ls.inner.Drain(sessionID, maxLines)
	if err != nil {
		ls.reset()
		return nil, err
	}
	return msgs, nil
}

func (ls *lazyStore) Status() (map[string]int, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if err := ls.ensureConnected(); err != nil {
		return nil, err
	}
	counts, err := ls.inner.Status()
	if err != nil {
		ls.reset()
		return nil, err
	}
	return counts, nil
}

func (ls *lazyStore) Write(_ string, _ store.LogMessage) error {
	return errors.New("lazyStore: write not supported (read-only)")
}

func (ls *lazyStore) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.closed = true
	if ls.inner != nil {
		err := ls.inner.Close()
		ls.inner = nil
		return err
	}
	return nil
}
