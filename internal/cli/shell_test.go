package cli

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestAvailableShells(t *testing.T) {
	tests := []struct {
		name   string
		shells map[string]config.Shell
		want   string
	}{
		{"empty shells", map[string]config.Shell{}, "(none)"},
		{"single shell", map[string]config.Shell{"default": {}}, "default"},
		{
			"multiple shells are sorted",
			map[string]config.Shell{"minimal": {}, "extended": {}, "default": {}},
			"default, extended, minimal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := availableShells(tt.shells); got != tt.want {
				t.Errorf("availableShells() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShellCmd_DefaultName(t *testing.T) {
	cmd := &ShellCmd{}

	// Verify default values
	if cmd.Name != "" {
		t.Errorf("expected empty Name by default, got %q", cmd.Name)
	}
	if cmd.Command != "" {
		t.Errorf("expected empty Command by default, got %q", cmd.Command)
	}
}
