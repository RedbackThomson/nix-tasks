//go:build integration

package integration_test

import (
	"strings"
	"testing"
)

func TestValidate_CountsPackageNames(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  packages = { a = pkgs.hello; b = pkgs.jq; };
  tasks.t = { deps = [ "a" ]; commands = [ "true" ]; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "validate", "-f", dir)
	if code != 0 {
		t.Fatalf("validate exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Packages: 2") {
		t.Errorf("expected package count, got: %s", stdout)
	}
}
