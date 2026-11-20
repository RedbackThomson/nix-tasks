//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList_ReportsConfigDecodeError(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.web = lib.mkTask { commands = [ "true" ]; env = { PORT = 8080; }; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code == 0 {
		t.Fatalf("expected list to fail on a non-string env value, got stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "tasks.web") {
		t.Errorf("expected error to name the task, got: %s", stderr)
	}
}

func TestList_FallsBackToSystemIndependentConfig(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	root, err := filepath.Abs(filepath.Join(testdataDir(t), "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	flake := fmt.Sprintf(`{
  inputs.nix-tasks.url = "path:%s";
  outputs = { self, nix-tasks }: {
    nixTasksConfig = { tasks.flat-task = { description = "From flat config"; commands = [ "true" ]; }; };
  };
}
`, root)
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), []byte(flake), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code != 0 {
		t.Fatalf("list exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "flat-task") {
		t.Errorf("expected task from system-independent config, got: %s", stdout)
	}
}
