//go:build integration

package integration_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const signalTasksConfig = `{
  tasks = {
    slow = tasksLib.mkTask {
      commands = [
        "sleep 60 & echo $! > \"$NIX_TASKS_TEST_LOG_DIR/child.pid\""
        "wait"
      ];
    };
    after-slow = tasksLib.mkTask {
      depends = [ "task:slow" ];
      commands = [ "touch \"$NIX_TASKS_TEST_LOG_DIR/after-slow.ran\"" ];
    };
  };
}`

// startNixTasks starts nix-tasks in the background with the same environment
// as runNixTasksInDir.
func startNixTasks(t *testing.T, binary, logDir string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = logDir
	cmd.Env = append(cmd.Environ(), "NIX_TASKS_CACHE_DIR="+t.TempDir(), "NIX_TASKS_TEST_LOG_DIR="+logDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, &out
}

// waitExit waits for cmd and returns its exit code, failing if it takes
// longer than limit.
func waitExit(t *testing.T, cmd *exec.Cmd, limit time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		return 0
	case <-time.After(limit):
		t.Fatalf("nix-tasks did not exit within %v of the signal", limit)
		return -1
	}
}

func waitForFile(t *testing.T, path string, limit time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
			return strings.TrimSpace(string(data))
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear within %v", path, limit)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRun_SignalStopsTasksAndExits130(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, signalTasksConfig, map[string]string{})
	logDir := t.TempDir()

	cmd, out := startNixTasks(t, binary, logDir, "run", "after-slow", "-f", dir)
	pid, err := strconv.Atoi(waitForFile(t, filepath.Join(logDir, "child.pid"), 90*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := waitExit(t, cmd, 10*time.Second); code != 130 {
		t.Errorf("exit code %d, want 130\noutput: %s", code, out)
	}

	assertStatuses(t, out.String(), map[string]string{"slow": "interrupted", "after-slow": "skipped"})
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("task's child process %d survived the interrupt", pid)
	}
	if _, err := os.Stat(filepath.Join(logDir, "after-slow.ran")); err == nil {
		t.Error("after-slow ran after the run was interrupted")
	}
}

func TestRun_SignalDuringConfigLoadExits130(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, signalTasksConfig, map[string]string{})
	logDir := t.TempDir()

	cmd, out := startNixTasks(t, binary, logDir, "run", "slow", "-f", dir)
	time.Sleep(100 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := waitExit(t, cmd, 10*time.Second); code != 130 {
		t.Errorf("exit code %d, want 130\noutput: %s", code, out)
	}
}

func TestRun_SecondSignalKillsImmediately(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, `{
  tasks.stubborn = tasksLib.mkTask {
    commands = [
      "trap '' TERM"
      "sleep 60 & echo $! > \"$NIX_TASKS_TEST_LOG_DIR/child.pid\""
      "wait"
    ];
  };
}`, map[string]string{})
	logDir := t.TempDir()

	cmd, out := startNixTasks(t, binary, logDir, "run", "stubborn", "-f", dir)
	pid, err := strconv.Atoi(waitForFile(t, filepath.Join(logDir, "child.pid"), 90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// The task ignores SIGTERM, so only an immediate exit on the second
	// signal gets in well under the kill grace period.
	if code := waitExit(t, cmd, 2*time.Second); code != -1 {
		t.Errorf("expected nix-tasks to be killed by the second signal, exited %d\noutput: %s", code, out)
	}
}
