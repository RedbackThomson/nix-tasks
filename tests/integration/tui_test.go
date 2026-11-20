//go:build integration

package integration_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runInPTY runs binary under script(1) so it sees a terminal on stdout and
// returns everything it wrote to that terminal.
func runInPTY(t *testing.T, binary string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "script", append([]string{"-q", "/dev/null", binary}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, "script", "-qec", shellQuote(append([]string{binary}, args...)), "/dev/null")
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%v timed out; output: %q", args, out)
	}
	if err != nil {
		t.Fatalf("script %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func shellQuote(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// Commands other than the TUI must not query the terminal: the replies leak
// into the shell prompt when nix-tasks exits before reading them.
func TestTUI_NoTerminalQueriesOutsideTUI(t *testing.T) {
	out := runInPTY(t, nixTasksBinary(t), "--help")

	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected help output, got %q", out)
	}
	for _, query := range []string{"\x1b]11;?", "\x1b[6n"} {
		if strings.Contains(out, query) {
			t.Errorf("expected no terminal query %q, got %q", query, out)
		}
	}
}

func TestTUI_PipedPrintsList(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	bare, stderr, err := runNixTasks(t, binary, "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks failed: %v\nstderr: %s", err, stderr)
	}
	list, stderr, err := runNixTasks(t, binary, "list", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks list failed: %v\nstderr: %s", err, stderr)
	}
	if bare != list {
		t.Errorf("expected piped nix-tasks to print the list\ngot:\n%s\nwant:\n%s", bare, list)
	}
	if !strings.Contains(bare, "hello") {
		t.Errorf("expected the hello task in output, got: %s", bare)
	}
}

func TestRun_SeveralTasksInOneRun(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "run", "hello", "goodbye", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	assertStatuses(t, stdout, map[string]string{"hello": "passed", "goodbye": "passed"})
	if n := strings.Count(stdout, "Completed:"); n != 1 {
		t.Errorf("expected one summary, got %d:\n%s", n, stdout)
	}

	_, stderr, err = runNixTasks(t, binary, "run", "hello", "nope", "-f", flakePath)
	assertExitCode(t, err, 1)
	if !strings.Contains(stderr, "task not found: nope") {
		t.Errorf("expected the unknown task to be named, got: %s", stderr)
	}
}
