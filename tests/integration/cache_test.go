//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A cached task must re-run when one of its packages changes, even though the
// config only carries package names.
func TestRun_CacheInvalidatedByPackageChange(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	config := func(pkg string) string {
		return `{
  packages.tool = pkgs.` + pkg + `;
  tasks.t = { deps = [ "tool" ]; inputs = [ "in.txt" ]; commands = [ "true" ]; };
}`
	}
	dir := writeTempFlake(t, config("hello"), map[string]string{"in.txt": "x"})
	other := writeTempFlake(t, config("jq"), map[string]string{"in.txt": "x"})
	cacheDir := t.TempDir()

	run := func() string {
		t.Helper()
		stdout, stderr, err := runNixTasksInDir(t, binary, dir, cacheDir, "run", "t", "-f", dir)
		if err != nil {
			t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		return stdout
	}

	run()
	if out := run(); !strings.Contains(out, "(cached)") {
		t.Fatalf("expected second run to be cached, got: %s", out)
	}

	flake, err := os.ReadFile(filepath.Join(other, "flake.nix"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), flake, 0o644); err != nil {
		t.Fatal(err)
	}
	if out := run(); strings.Contains(out, "(cached)") {
		t.Errorf("expected run after package change to execute, got: %s", out)
	}
}
