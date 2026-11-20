package cli

import (
	"errors"

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

// exitInterrupted is the conventional exit code for a run stopped by SIGINT.
const exitInterrupted = 130

var errInterrupted = &ExitCodeError{Code: exitInterrupted, Err: errors.New("interrupted")}

// runExitCode picks the process exit code for a run: 0 when nothing failed.
// Only an unambiguous failure (a single task whose process exited non-zero)
// passes its code through, so callers and CI see e.g. a test runner's own
// exit status. Real failures outrank interruption, since a fail-fast run
// interrupts the siblings of the task that failed.
func runExitCode(results []runner.TaskResult) int {
	var failed []runner.TaskResult
	interrupted := false
	for _, r := range results {
		switch r.State {
		case runner.Failed:
			failed = append(failed, r)
		case runner.Interrupted:
			interrupted = true
		}
	}
	switch {
	case len(failed) == 1:
		if code := failed[0].ExitCode(); code > 0 {
			return code
		}
		return 1
	case len(failed) > 1:
		return 1
	case interrupted:
		return exitInterrupted
	}
	return 0
}
