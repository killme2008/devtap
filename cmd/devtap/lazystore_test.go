package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

// mockStore is a minimal store.Store for testing lazyStore behavior.
type mockStore struct {
	drainFn  func(string, int) ([]store.LogMessage, error)
	statusFn func() (map[string]int, error)
	closed   bool
}

func (m *mockStore) Write(string, store.LogMessage) error { return nil }
func (m *mockStore) Close() error                         { m.closed = true; return nil }

func (m *mockStore) Drain(s string, n int) ([]store.LogMessage, error) {
	return m.drainFn(s, n)
}

func (m *mockStore) Status() (map[string]int, error) {
	return m.statusFn()
}

func TestLazyStore_FirstCallConnects(t *testing.T) {
	connectCalls := 0
	ms := &mockStore{
		drainFn:  func(string, int) ([]store.LogMessage, error) { return nil, nil },
		statusFn: func() (map[string]int, error) { return map[string]int{"s": 0}, nil },
	}
	ls := newLazyStore(func() (store.Store, error) {
		connectCalls++
		return ms, nil
	})

	_, err := ls.Drain("s", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if connectCalls != 1 {
		t.Fatalf("expected 1 connect call, got %d", connectCalls)
	}

	// Second call should reuse cached connection.
	_, err = ls.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if connectCalls != 1 {
		t.Fatalf("expected still 1 connect call, got %d", connectCalls)
	}
}

func TestLazyStore_CooldownPreventsRapidRetry(t *testing.T) {
	connectCalls := 0
	ls := newLazyStore(func() (store.Store, error) {
		connectCalls++
		return nil, errors.New("connection refused")
	})
	ls.cooldown = 1 * time.Hour // very long cooldown

	// First call: attempts connection, fails.
	_, err := ls.Drain("s", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if connectCalls != 1 {
		t.Fatalf("expected 1 connect call, got %d", connectCalls)
	}

	// Second call within cooldown: should NOT retry.
	_, err = ls.Drain("s", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if connectCalls != 1 {
		t.Fatalf("expected still 1 connect call (cooldown), got %d", connectCalls)
	}
}

func TestLazyStore_RetriesAfterCooldown(t *testing.T) {
	connectCalls := 0
	ls := newLazyStore(func() (store.Store, error) {
		connectCalls++
		return nil, errors.New("connection refused")
	})
	ls.cooldown = 1 * time.Millisecond

	// First call fails.
	_, _ = ls.Drain("s", 10)
	if connectCalls != 1 {
		t.Fatalf("expected 1 connect call, got %d", connectCalls)
	}

	time.Sleep(5 * time.Millisecond)

	// After cooldown: should retry.
	_, _ = ls.Drain("s", 10)
	if connectCalls != 2 {
		t.Fatalf("expected 2 connect calls after cooldown, got %d", connectCalls)
	}
}

func TestLazyStore_DrainErrorTriggersReset(t *testing.T) {
	connectCalls := 0
	drainErr := true
	ms := &mockStore{
		drainFn: func(string, int) ([]store.LogMessage, error) {
			if drainErr {
				return nil, errors.New("query failed")
			}
			return nil, nil
		},
		statusFn: func() (map[string]int, error) { return nil, nil },
	}
	ls := newLazyStore(func() (store.Store, error) {
		connectCalls++
		return ms, nil
	})
	ls.cooldown = 1 * time.Millisecond

	// First call: connects successfully, Drain fails → reset.
	_, err := ls.Drain("s", 10)
	if err == nil {
		t.Fatal("expected drain error")
	}
	if connectCalls != 1 {
		t.Fatalf("expected 1 connect call, got %d", connectCalls)
	}
	if !ms.closed {
		t.Fatal("expected inner store to be closed after reset")
	}

	time.Sleep(5 * time.Millisecond)

	// Provide a fresh mock for the reconnect.
	ms2 := &mockStore{
		drainFn:  func(string, int) ([]store.LogMessage, error) { return nil, nil },
		statusFn: func() (map[string]int, error) { return nil, nil },
	}
	drainErr = false
	ls.mu.Lock()
	ls.connectFn = func() (store.Store, error) {
		connectCalls++
		return ms2, nil
	}
	ls.mu.Unlock()

	// After cooldown: reconnects.
	_, err = ls.Drain("s", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if connectCalls != 2 {
		t.Fatalf("expected 2 connect calls, got %d", connectCalls)
	}
}

func TestLazyStore_StatusErrorTriggersReset(t *testing.T) {
	ms := &mockStore{
		drainFn: func(string, int) ([]store.LogMessage, error) { return nil, nil },
		statusFn: func() (map[string]int, error) {
			return nil, errors.New("status failed")
		},
	}
	ls := newLazyStore(func() (store.Store, error) {
		return ms, nil
	})
	ls.cooldown = 1 * time.Millisecond

	_, err := ls.Status()
	if err == nil {
		t.Fatal("expected status error")
	}
	if !ms.closed {
		t.Fatal("expected inner store to be closed after reset")
	}
}

func TestLazyStore_WriteReturnsError(t *testing.T) {
	ls := newLazyStore(func() (store.Store, error) {
		return nil, nil
	})
	err := ls.Write("s", store.LogMessage{})
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestLazyStore_CloseWhenNotConnected(t *testing.T) {
	ls := newLazyStore(func() (store.Store, error) {
		return nil, errors.New("nope")
	})
	// Should not panic or error.
	if err := ls.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Test that after reset, if the original connection was established > cooldown ago,
// the next call retries immediately (no extra wait). This is because reset() does
// NOT update lastAttempt — the elapsed time since the original connect is already
// past the cooldown window.
func TestLazyStore_ResetAllowsImmediateRetry(t *testing.T) {
	connectCalls := 0
	failDrain := true

	ls := newLazyStore(func() (store.Store, error) {
		connectCalls++
		return &mockStore{
			drainFn: func(string, int) ([]store.LogMessage, error) {
				if failDrain {
					return nil, errors.New("query failed")
				}
				return []store.LogMessage{{Tag: "ok"}}, nil
			},
			statusFn: func() (map[string]int, error) { return nil, nil },
		}, nil
	})
	ls.cooldown = 50 * time.Millisecond

	// Connect succeeds, Drain fails → reset.
	_, err := ls.Drain("s", 10)
	if err == nil {
		t.Fatal("expected drain error")
	}
	if connectCalls != 1 {
		t.Fatalf("expected 1 connect call, got %d", connectCalls)
	}

	// Wait longer than cooldown so that time.Since(lastAttempt) > cooldown.
	time.Sleep(60 * time.Millisecond)

	// Now Drain succeeds. The retry should happen immediately (no extra cooldown)
	// because lastAttempt was set during the original connect, >50ms ago.
	failDrain = false
	msgs, err := ls.Drain("s", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if connectCalls != 2 {
		t.Fatalf("expected 2 connect calls (immediate retry after reset), got %d", connectCalls)
	}
	if len(msgs) != 1 || msgs[0].Tag != "ok" {
		t.Fatalf("unexpected messages: %v", msgs)
	}
}

func TestLazyStore_ConcurrentAccess(t *testing.T) {
	var connectCalls atomic.Int32
	ls := newLazyStore(func() (store.Store, error) {
		connectCalls.Add(1)
		return &mockStore{
			drainFn: func(string, int) ([]store.LogMessage, error) {
				return []store.LogMessage{{Tag: "test"}}, nil
			},
			statusFn: func() (map[string]int, error) {
				return map[string]int{"s": 1}, nil
			},
		}, nil
	})
	ls.cooldown = 1 * time.Millisecond

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, _ = ls.Drain("s", 10)
			} else {
				_, _ = ls.Status()
			}
		}(i)
	}

	wg.Wait()

	// Should have connected exactly once (all goroutines share the cached connection).
	if c := connectCalls.Load(); c != 1 {
		t.Fatalf("expected 1 connect call, got %d", c)
	}
}

func TestLazyStore_DrainAfterCloseReturnsError(t *testing.T) {
	ms := &mockStore{
		drainFn:  func(string, int) ([]store.LogMessage, error) { return nil, nil },
		statusFn: func() (map[string]int, error) { return nil, nil },
	}
	ls := newLazyStore(func() (store.Store, error) {
		return ms, nil
	})

	// Connect, then close, then try to drain.
	_, _ = ls.Drain("s", 10)
	_ = ls.Close()

	_, err := ls.Drain("s", 10)
	if err == nil {
		t.Fatal("expected error after close")
	}
	_, err = ls.Status()
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestLazyStore_CloseDelegatesToInner(t *testing.T) {
	ms := &mockStore{
		drainFn:  func(string, int) ([]store.LogMessage, error) { return nil, nil },
		statusFn: func() (map[string]int, error) { return nil, nil },
	}
	ls := newLazyStore(func() (store.Store, error) {
		return ms, nil
	})

	// Force connect.
	_, _ = ls.Drain("s", 10)
	if ms.closed {
		t.Fatal("inner should not be closed yet")
	}

	if err := ls.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ms.closed {
		t.Fatal("expected inner store to be closed")
	}
}
