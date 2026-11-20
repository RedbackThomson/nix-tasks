//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const locationTasksConfig = `{
  tasks = {
    where = tasksLib.mkTask { commands = [ "pwd -P" ]; };
    where-sub = tasksLib.mkTask { workingDir = "sub"; commands = [ "pwd -P" ]; };
    artifact = {
      type = "build";
      derivation = pkgs.runCommand "flake-root-location" { } "echo ${builtins.readFile ./marker} > $out";
    };
  };
}`

func TestRun_FromSubdirectoryUsesFlakeRoot(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, locationTasksConfig, map[string]string{
		"marker":           randomString(t),
		"sub/deeper/.keep": "",
	})
	deeper := filepath.Join(dir, "sub", "deeper")

	stdout, stderr, code := runNixTasksIn(t, binary, deeper, "run", "--raw", "where")
	if code != 0 {
		t.Fatalf("run where exited %d\nstderr: %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != dir {
		t.Errorf("task without workingDir ran in %q, want flake root %q", got, dir)
	}

	stdout, stderr, code = runNixTasksIn(t, binary, deeper, "run", "--raw", "where-sub")
	if code != 0 {
		t.Fatalf("run where-sub exited %d\nstderr: %s", code, stderr)
	}
	if got, want := strings.TrimSpace(stdout), filepath.Join(dir, "sub"); got != want {
		t.Errorf("task with workingDir ran in %q, want %q", got, want)
	}

	stdout, stderr, code = runNixTasksIn(t, binary, deeper, "run", "artifact")
	if code != 0 {
		t.Fatalf("run artifact exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nix-tasks", "artifact", "out")); err != nil {
		t.Errorf("build output not linked in flake root: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(deeper, ".nix-tasks")); err == nil {
		t.Errorf("build output linked in the current directory instead of the flake root")
	}
}

func TestList_RelativeFlakePath(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, locationTasksConfig, map[string]string{"marker": "x"})

	stdout, stderr, code := runNixTasksIn(t, binary, filepath.Dir(dir), "list", "-f", filepath.Base(dir))
	if code != 0 {
		t.Fatalf("list -f %s exited %d\nstderr: %s", filepath.Base(dir), code, stderr)
	}
	if !strings.Contains(stdout, "where-sub") {
		t.Errorf("expected tasks from relative flake path, got: %s", stdout)
	}
}
