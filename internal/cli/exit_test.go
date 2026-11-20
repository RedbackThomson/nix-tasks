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

func TestRunExitCode(t *testing.T) {
	failed := func(name string, code int) runner.TaskResult {
		err := &runner.TaskExecutionError{Name: name, Err: exitErrorWithCode(t, code)}
		return runner.TaskResult{Name: name, State: runner.Failed, Error: err}
	}

	tests := []struct {
		name    string
		results []runner.TaskResult
		want    int
	}{
		{
			name:    "all succeeded",
			results: []runner.TaskResult{{Name: "ok", State: runner.Succeeded}},
			want:    0,
		},
		{
			name: "single failure passes its exit code through wrapping",
			results: []runner.TaskResult{
				{Name: "ok", State: runner.Succeeded},
				failed("t", 4),
			},
			want: 4,
		},
		{
			name: "skipped and interrupted tasks do not count as failures",
			results: []runner.TaskResult{
				failed("t", 3),
				{Name: "skipped", State: runner.Skipped},
				{Name: "killed", State: runner.Interrupted, Error: context.Canceled},
			},
			want: 3,
		},
		{
			name:    "multiple failures exit 1",
			results: []runner.TaskResult{failed("a", 4), failed("b", 5)},
			want:    1,
		},
		{
			name: "failure without a process exit code exits 1",
			results: []runner.TaskResult{
				{Name: "a", State: runner.Failed, Error: errors.New("nix build failed")},
			},
			want: 1,
		},
		{
			name: "allowed failures do not fail the run",
			results: []runner.TaskResult{
				{Name: "a", State: runner.FailedAllowed, Error: errors.New("boom")},
				{Name: "b", State: runner.Succeeded},
			},
			want: 0,
		},
		{
			name: "interruption exits 130",
			results: []runner.TaskResult{
				{Name: "a", State: runner.Interrupted, Error: context.Canceled},
				{Name: "b", State: runner.Skipped},
			},
			want: 130,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runExitCode(tt.results); got != tt.want {
				t.Errorf("runExitCode() = %d, want %d", got, tt.want)
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
