package runner_test

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestGenerateScript(t *testing.T) {
	tests := []struct {
		name string
		task config.Task
		want string
	}{
		{
			name: "single command",
			task: config.Task{
				Commands: []string{"go build ./..."},
			},
			want: "go build ./...",
		},
		{
			name: "multiple commands",
			task: config.Task{
				Commands: []string{"go build ./...", "go test ./..."},
			},
			want: "go build ./...\ngo test ./...",
		},
		{
			name: "raw script takes precedence",
			task: config.Task{
				Commands: []string{"go build ./..."},
				Script:   "#!/bin/bash\necho hello",
			},
			want: "#!/bin/bash\necho hello",
		},
		{
			name: "empty task",
			task: config.Task{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runner.GenerateScript(tt.task)
			if got != tt.want {
				t.Errorf("GenerateScript() = %q, want %q", got, tt.want)
			}
		})
	}
}
