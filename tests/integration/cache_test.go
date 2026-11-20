//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A cached task must re-run when one of its packages changes, even though the
// config only carries package names.
func TestRun_CacheInvalidatedByPackageChange(t *testing.T) {
	skipIfNoNix(t)

	config := func(pkg string) string {
		return `{
  packages.tool = pkgs.` + pkg + `;
  tasks.t = { deps = [ "tool" ]; inputs = [ "in.txt" ]; commands = [ "true" ]; };
}`
	}
	dir := writeTempFlake(t, config("hello"), map[string]string{"in.txt": "x"})
	other := writeTempFlake(t, config("jq"), map[string]string{"in.txt": "x"})
	cacheDir := t.TempDir()

	run := func() bool {
		t.Helper()
		r := runJSON(t, dir, cacheDir, "run", "t", "-f", dir)
		r.assertCode(t, 0)
		return r.task(t, "t").Cached
	}

	run()
	if !run() {
		t.Fatal("expected second run to be cached")
	}

	flake, err := os.ReadFile(filepath.Join(other, "flake.nix"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), flake, 0o644); err != nil {
		t.Fatal(err)
	}
	if run() {
		t.Error("expected run after package change to execute")
	}
}
