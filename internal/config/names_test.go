package config

import (
	"strings"
	"testing"
)

func TestNameErrors(t *testing.T) {
	cfg := &Config{
		Tasks: map[string]Task{
			`lint.go`:   {},
			`a\b ${x}%`: {},
			`say"hi"`:   {},
		},
		DevShells: map[string]Shell{
			`ci.full`:  {},
			`"quoted"`: {},
		},
	}
	errs := nameErrors(cfg)
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{`task 'say"hi"'`, `shell '"quoted"'`, "rename the task", "rename the shell"} {
		if !strings.Contains(joined, want) {
			t.Errorf("errors %q do not mention %q", joined, want)
		}
	}
}
