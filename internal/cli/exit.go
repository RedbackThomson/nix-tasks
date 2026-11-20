package cli

import (
	"context"
	"errors"
	"os/exec"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// ExitCodeError is returned when nix-tasks should exit with a specific code
// rather than the generic failure code 1.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

// failureExitCode picks the process exit code for a failed run. Only an
// unambiguous failure (a single task whose process exited non-zero) passes its
// code through, so callers and CI see e.g. a test runner's own exit status.
func failureExitCode(results []runner.TaskResult) int {
	var failed []runner.TaskResult
	for _, r := range results {
		if !r.Success && !errors.Is(r.Error, context.Canceled) {
			failed = append(failed, r)
		}
	}
	if len(failed) != 1 {
		return 1
	}

	var exitErr *exec.ExitError
	if errors.As(failed[0].Error, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return 1
}
