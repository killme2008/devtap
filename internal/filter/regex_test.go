package filter

import "testing"

func TestRegex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		invert  bool
		input   string
		want    bool
	}{
		{"match error", "error|warning", false, "error[E0308]: mismatched types", true},
		{"match warning", "error|warning", false, "warning: unused variable", true},
		{"no match", "error|warning", false, "Compiling myproject v0.1.0", false},
		{"inverted match", "Compiling|Finished", true, "error: something", true},
		{"inverted no match", "Compiling|Finished", true, "Compiling myproject", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Regex(tt.pattern, tt.invert)
			if err != nil {
				t.Fatalf("Regex: %v", err)
			}
			got := f(tt.input)
			if got != tt.want {
				t.Errorf("filter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRegexEmpty(t *testing.T) {
	f, err := Regex("", false)
	if err != nil {
		t.Fatalf("Regex: %v", err)
	}
	if f != nil {
		t.Error("expected nil filter for empty pattern")
	}
}

func TestRegexInvalid(t *testing.T) {
	_, err := Regex("[invalid", false)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
