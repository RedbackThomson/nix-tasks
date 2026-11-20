//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const outputTasksConfig = `{
  tasks = {
    base = tasksLib.mkTask { commands = [ "echo BASE-FAILED" "exit 3" ]; };
    left = tasksLib.mkTask { depends = [ "task:base" ]; commands = [ "true" ]; };
    right = tasksLib.mkTask { depends = [ "task:base" ]; commands = [ "true" ]; };
    top = tasksLib.mkTask { depends = [ "task:left" "task:right" ]; commands = [ "true" ]; };

    quiet = tasksLib.mkTask { commands = [ "echo QUIET-OUTPUT" ]; };
    split = tasksLib.mkTask { commands = [ "printf 'Downloading...'; sleep 1; printf ' done\\n'" ]; };
    long = tasksLib.mkTask { commands = [ "for i in $(seq 1 80); do echo long-line-$i; done" "exit 1" ]; };
    target = tasksLib.mkTask { depends = [ "task:long" ]; commands = [ "echo never" ]; };
    long-allowed = tasksLib.mkTask {
      continueOnError = true;
      commands = [ "for i in $(seq 1 80); do echo allowed-line-$i; done" "exit 1" ];
    };
  };
}`

// runOutput runs nix-tasks with stdout piped, outside CI, with a private
// TMPDIR, plus any extra environment.
func runOutput(t *testing.T, dir, tmp string, env []string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, nixTasksBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(testEnviron("CI", "NO_COLOR", "TMPDIR"), "NIX_TASKS_CACHE_DIR="+t.TempDir(), "TMPDIR="+tmp)
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

func logFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "nix-tasks-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestOutput_PipedShowsProgressAndFailureOnce(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})
	tmp := t.TempDir()

	stdout, _, code := runOutput(t, dir, tmp, nil, "run", "top")
	if code != 3 {
		t.Errorf("exit code %d, want 3", code)
	}
	if strings.Contains(stdout, "\x1b") {
		t.Errorf("piped output should have no escape sequences:\n%q", stdout)
	}
	if !strings.Contains(stdout, "▶ base\n✗ base (") || !strings.Contains(stdout, "(exit code 3)\n") {
		t.Errorf("expected start and finish lines for base:\n%s", stdout)
	}
	r := runJSON(t, t.TempDir(), t.TempDir(), "run", "top", "-f", dir)
	r.assertCode(t, 3)
	r.assertStates(t, map[string]string{"base": "failed", "left": "skipped", "right": "skipped", "top": "skipped"})
	if n := strings.Count(stdout, "BASE-FAILED"); n != 1 {
		t.Errorf("failed output shown %d times, want once:\n%s", n, stdout)
	}
	// The summary tree draws base under left, then marks it as already shown
	// under right.
	tree := `
top - (skipped)
├─ left - (skipped)
│  └─ base ✗ (`
	if !strings.Contains(stdout, tree) || !strings.Contains(stdout, "\n└─ right - (skipped)\n   └─ base ✗ (") || !strings.Contains(stdout, "(exit code 3) *\n") {
		t.Errorf("expected the dependency tree in the summary:\n%s", stdout)
	}
	if !strings.HasSuffix(stdout, "Completed: 4 tasks (1 failed, 3 skipped)\n") {
		t.Errorf("unexpected summary:\n%s", stdout)
	}
	if logs := logFiles(t, tmp); len(logs) != 0 {
		t.Errorf("short output should not be logged, found %v", logs)
	}
}

func TestOutput_SuccessfulOutputHiddenWhenPiped(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})
	tmp := t.TempDir()

	stdout, stderr, code := runOutput(t, dir, tmp, nil, "run", "quiet")
	if code != 0 {
		t.Fatalf("exit code %d\n%s", code, stderr)
	}
	if strings.Contains(stdout, "QUIET-OUTPUT") {
		t.Errorf("successful output should be hidden when piped:\n%s", stdout)
	}
	if logs := logFiles(t, tmp); len(logs) != 0 {
		t.Errorf("successful run left logs %v", logs)
	}
}

func TestOutput_Color(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})

	tests := []struct {
		name      string
		env       []string
		args      []string
		wantColor bool
	}{
		{"auto when piped", nil, []string{"run", "quiet"}, false},
		{"always", nil, []string{"--color=always", "run", "quiet"}, true},
		{"always after the subcommand", nil, []string{"run", "quiet", "--color", "always"}, true},
		{"always beats NO_COLOR", []string{"NO_COLOR=1"}, []string{"--color=always", "run", "quiet"}, true},
		{"never", nil, []string{"--color=never", "run", "quiet"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runOutput(t, dir, t.TempDir(), tt.env, tt.args...)
			if code != 0 {
				t.Fatalf("exit code %d\n%s", code, stderr)
			}
			if got := strings.Contains(stdout, "\x1b["); got != tt.wantColor {
				t.Errorf("colour = %v, want %v:\n%q", got, tt.wantColor, stdout)
			}
		})
	}
}

func TestOutput_StreamKeepsLinesWhole(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})

	for _, flag := range []string{"--stream", "-v"} {
		stdout, stderr, code := runOutput(t, dir, t.TempDir(), nil, "run", "split", flag)
		if code != 0 {
			t.Fatalf("%s: exit code %d\n%s", flag, code, stderr)
		}
		if !strings.Contains(stdout, "[split] Downloading... done\n") {
			t.Errorf("%s: expected one whole prefixed line:\n%s", flag, stdout)
		}
	}
}

func TestOutput_LongFailureSavedToLog(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})
	tmp := t.TempDir()

	stdout, _, _ := runOutput(t, dir, tmp, nil, "run", "long")
	logs := logFiles(t, tmp)
	if len(logs) != 1 {
		t.Fatalf("expected one log file, found %v", logs)
	}
	if !strings.Contains(stdout, "... 30 lines omitted ...") || !strings.Contains(stdout, "full output: "+logs[0]) {
		t.Errorf("expected truncated tail and log path:\n%s", stdout)
	}
	if strings.Contains(stdout, "long-line-30\n") || !strings.Contains(stdout, "long-line-31\n") {
		t.Errorf("expected the last 50 lines:\n%s", stdout)
	}
	data, err := os.ReadFile(logs[0])
	if err != nil || !strings.Contains(string(data), "long-line-1\n") {
		t.Errorf("log should hold the full output: %v", err)
	}
}

func TestOutput_AllowedFailureWritesNoLog(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})
	tmp := t.TempDir()

	stdout, stderr, code := runOutput(t, dir, tmp, nil, "run", "long-allowed")
	if code != 0 {
		t.Fatalf("exit code %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "... 30 lines omitted ...") || !strings.Contains(stdout, "allowed-line-80\n") {
		t.Errorf("expected the allowed failure's output tail:\n%s", stdout)
	}
	if logs := logFiles(t, tmp); len(logs) != 0 || strings.Contains(stdout, "full output") {
		t.Errorf("an allowed failure should not write a log, found %v:\n%s", logs, stdout)
	}
}

func TestOutput_RawNamesFailedDependency(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, outputTasksConfig, map[string]string{})
	tmp := t.TempDir()

	stdout, stderr, code := runOutput(t, dir, tmp, nil, "run", "--raw", "target")
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("raw mode should print nothing to stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "target - (skipped)\n└─ long ✗ (") || !strings.Contains(stderr, "long-line-80") {
		t.Errorf("expected failed dependency and its output tail on stderr:\n%s", stderr)
	}
	if logs := logFiles(t, tmp); len(logs) != 1 || !strings.Contains(stderr, "full output: "+logs[0]) {
		t.Errorf("expected the full output log to be named, logs %v:\n%s", logs, stderr)
	}
}
