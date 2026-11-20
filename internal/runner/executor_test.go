package runner

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestExecutor_TaskDir(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		workingDir string
		want       string
	}{
		{"no workingDir runs in flake root", "/repo", "", "/repo"},
		{"relative workingDir resolves against flake root", "/repo", "services/api", "/repo/services/api"},
		{"absolute workingDir is kept", "/repo", "/tmp/x", "/tmp/x"},
		{"no flake root keeps current directory", "", "", ""},
		{"no flake root keeps relative workingDir", "", "sub", "sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Executor{options: Options{FlakeRoot: tt.root}}
			if got := e.taskDir(config.Task{WorkingDir: tt.workingDir}); got != tt.want {
				t.Errorf("taskDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
