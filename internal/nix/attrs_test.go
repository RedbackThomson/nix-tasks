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

func TestParseBuildOutputs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "single output",
			input: `[{"drvPath":"/nix/store/x.drv","outputs":{"out":"/nix/store/aaa-app"}}]`,
			want:  map[string]string{"out": "/nix/store/aaa-app"},
		},
		{
			name:  "multiple outputs keep their names",
			input: `[{"drvPath":"/nix/store/x.drv","outputs":{"bin":"/nix/store/bbb-app-bin","out":"/nix/store/aaa-app"}}]`,
			want:  map[string]string{"out": "/nix/store/aaa-app", "bin": "/nix/store/bbb-app-bin"},
		},
		{name: "no outputs", input: `[]`, wantErr: true},
		{name: "not json", input: `/nix/store/aaa-app`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nix.ParseBuildOutputs([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("output %s = %s, want %s", k, got[k], v)
				}
			}
		})
	}
}
