package runner

import (
	"slices"
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

func TestWithTaskEnv(t *testing.T) {
	shell := []string{"PATH=/nix/store/go/bin:/usr/bin", "CC=clang", "HOME=/home/me"}
	tests := []struct {
		name    string
		env     []string
		taskEnv map[string]string
		wantVar map[string]string
	}{
		{"no task env keeps the shell's", shell, nil,
			map[string]string{"PATH": "/nix/store/go/bin:/usr/bin", "CC": "clang"}},
		{"PATH is prepended", shell, map[string]string{"PATH": "/repo/bin"},
			map[string]string{"PATH": "/repo/bin:/nix/store/go/bin:/usr/bin"}},
		{"other variables win outright", shell, map[string]string{"CC": "gcc", "NEW": "x"},
			map[string]string{"CC": "gcc", "NEW": "x", "PATH": "/nix/store/go/bin:/usr/bin"}},
		{"PATH without an existing one is used as is", []string{"HOME=/home/me"}, map[string]string{"PATH": "/repo/bin"},
			map[string]string{"PATH": "/repo/bin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withTaskEnv(slices.Clone(tt.env), tt.taskEnv)
			for k, want := range tt.wantVar {
				if v := envValue(got, k); v != want {
					t.Errorf("%s = %q, want %q", k, v, want)
				}
			}
		})
	}
}
