package claudecode

import (
	"testing"
)

func TestEncodeProjectDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/dennis/foo", "-Users-dennis-foo"},
		{"/home/user/project", "-home-user-project"},
	}
	for _, tt := range tests {
		got := encodeProjectDir(tt.input)
		if got != tt.want {
			t.Errorf("encodeProjectDir(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
