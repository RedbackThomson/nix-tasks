package cli

import (
	"bytes"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestPrintNames(t *testing.T) {
	cfg := &config.Config{
		Tasks:     map[string]config.Task{"test": {}, "build": {}, "lint.go": {}},
		DevShells: map[string]config.Shell{"default": {}, "ci": {}},
	}
	tests := []struct {
		name   string
		shells bool
		want   string
	}{
		{"tasks", false, "build\nlint.go\ntest\n"},
		{"shells", true, "ci\ndefault\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printNames(&buf, cfg, tt.shells)
			if got := buf.String(); got != tt.want {
				t.Errorf("printNames() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintNames_Empty(t *testing.T) {
	var buf bytes.Buffer
	printNames(&buf, &config.Config{}, false)
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}
