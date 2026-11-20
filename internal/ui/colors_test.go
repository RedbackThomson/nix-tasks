package ui

import (
	"os"
	"testing"
)

func TestResolveColor(t *testing.T) {
	// A file is never a terminal, so auto relies on the TTY check.
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tests := []struct {
		name    string
		mode    ColorMode
		noColor bool
		want    bool
	}{
		{"auto off when not a terminal", ColorAuto, false, false},
		{"always", ColorAlways, false, true},
		{"always beats NO_COLOR", ColorAlways, true, true},
		{"never", ColorNever, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noColor {
				t.Setenv("NO_COLOR", "1")
			}
			if got := ResolveColor(tt.mode, f); got != tt.want {
				t.Errorf("ResolveColor(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	defer SetColor(ColorEnabled())
	SetColor(false)
	if got := Failure("x"); got != "x" {
		t.Errorf("Red with colour off = %q", got)
	}
	SetColor(true)
	if got := Failure("x"); got != "\x1b[31mx\x1b[0m" {
		t.Errorf("Red with colour on = %q", got)
	}
}

func TestColorSGR(t *testing.T) {
	for c, want := range map[Color]string{FailureColor: "31", AccentColor: "34", MutedColor: "90", 14: "96"} {
		if got := c.sgr(); got != want {
			t.Errorf("Color(%d).sgr() = %q, want %q", c, got, want)
		}
	}
}
