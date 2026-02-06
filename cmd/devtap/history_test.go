package main

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		d, err := parseDuration(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("parseDuration(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q): %v", tt.input, err)
			continue
		}
		if d != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, d, tt.want)
		}
	}
}
