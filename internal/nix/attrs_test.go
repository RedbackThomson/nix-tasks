package nix_test

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

func TestAttrHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"shell", nix.NixTasksShellsAttr("x86_64-linux", "build"), `nixTasksShells."x86_64-linux"."build"`},
		{"shell with dot", nix.NixTasksShellsAttr("x86_64-linux", "lint.go"), `nixTasksShells."x86_64-linux"."lint.go"`},
		{"build with dot", nix.NixTasksBuildsAttr("aarch64-darwin", "app.bin"), `nixTasksBuilds."aarch64-darwin"."app.bin"`},
		{"dev shell with dot", nix.DevShellAttr("aarch64-darwin", "ci.full"), `devShells."aarch64-darwin"."ci.full"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %s, want %s", tt.got, tt.want)
			}
		})
	}
}
