package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func exitErrorWithCode(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	return err
}

func TestFailureExitCode(t *testing.T) {
	wrapped := func(err error) error {
		return &runner.TaskExecutionError{Name: "t", Err: fmt.Errorf("task 't' failed: %w", err)}
	}

	tests := []struct {
		name    string
		results []runner.TaskResult
		want    int
	}{
		{
			name: "single failure passes its exit code through wrapping",
			results: []runner.TaskResult{
				{Name: "ok", Success: true},
				{Name: "t", Error: wrapped(exitErrorWithCode(t, 4))},
			},
			want: 4,
		},
		{
			name: "skipped tasks do not count as failures",
			results: []runner.TaskResult{
				{Name: "t", Error: wrapped(exitErrorWithCode(t, 3))},
				{Name: "skipped", Error: context.Canceled},
			},
			want: 3,
		},
		{
			name: "multiple failures exit 1",
			results: []runner.TaskResult{
				{Name: "a", Error: wrapped(exitErrorWithCode(t, 4))},
				{Name: "b", Error: wrapped(exitErrorWithCode(t, 5))},
			},
			want: 1,
		},
		{
			name: "failure without a process exit code exits 1",
			results: []runner.TaskResult{
				{Name: "a", Error: errors.New("failed (cached)")},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failureExitCode(tt.results); got != tt.want {
				t.Errorf("failureExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDevelopArgs(t *testing.T) {
	got := developArgs("/repo", `devShells."x86_64-linux"."default"`, "")
	want := []string{"nix", "develop", `/repo#devShells."x86_64-linux"."default"`}
	if !slices.Equal(got, want) {
		t.Errorf("interactive: got %q, want %q", got, want)
	}

	got = developArgs("/repo", `devShells."x86_64-linux"."ci"`, "exit 3")
	want = []string{"nix", "develop", `/repo#devShells."x86_64-linux"."ci"`, "--command", "bash", "-c", "exit 3"}
	if !slices.Equal(got, want) {
		t.Errorf("command: got %q, want %q", got, want)
	}
}
