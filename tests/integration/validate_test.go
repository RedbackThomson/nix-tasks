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

func TestValidate_ReportsCyclesAndUnbuildableTasks(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks = {
    a = { depends = [ "task:b" ]; commands = [ "true" ]; };
    b = { depends = [ "a" ]; commands = [ "true" ]; };
    image = { type = "build"; };
  };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "validate", "-f", dir)
	if code == 0 {
		t.Fatalf("expected validate to fail, got stdout: %s", stdout)
	}
	for _, want := range []string{
		"circular dependency detected: a -> b -> a",
		"task 'image': build task has no derivation",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, stderr)
		}
	}
}

func TestValidate_AcceptsResolvableBuildTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.hello = { type = "build"; derivation = pkgs.hello; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "validate", "-f", dir)
	if code != 0 {
		t.Fatalf("validate failed (%d)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Configuration is valid") {
		t.Errorf("unexpected output: %s", stdout)
	}
}
