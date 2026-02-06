package session

import (
	"testing"

	"github.com/killme2008/devtap/internal/adapter"
)

func TestPickSingle(t *testing.T) {
	sessions := []adapter.Session{
		{ID: "abc123", Label: "test session"},
	}

	id, err := Pick(sessions)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if id != "abc123" {
		t.Errorf("expected abc123, got %s", id)
	}
}

func TestPickEmpty(t *testing.T) {
	_, err := Pick(nil)
	if err == nil {
		t.Error("expected error for empty sessions")
	}
}
