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
		{
			name:   "empty shells",
			shells: map[string]config.Shell{},
			want:   "(none)",
		},
		{
			name: "single shell",
			shells: map[string]config.Shell{
				"default": {},
			},
			want: "default",
		},
		{
			name: "multiple shells",
			shells: map[string]config.Shell{
				"minimal":  {},
				"extended": {},
				"default":  {},
			},
			// Note: map iteration order is not guaranteed, so we just check length
			want: "", // Will check contains instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := availableShells(tt.shells)

			if tt.name == "multiple shells" {
				// For multiple shells, just verify all names are present
				for name := range tt.shells {
					if !contains(got, name) {
						t.Errorf("availableShells() = %v, missing shell %v", got, name)
					}
				}
			} else {
				if got != tt.want {
					t.Errorf("availableShells() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
