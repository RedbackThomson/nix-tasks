//go:build integration

package integration_test

import (
	"strings"
	"testing"
)

func TestShellTaskOutputsIsAnError(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.build = lib.mkTask {
    commands = [ "true" ];
    outputs = [ "bin/app" ];
  };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list")
	if code == 0 {
		t.Fatalf("expected list to fail, got success\nstdout: %s", stdout)
	}
	for _, want := range []string{"nix-tasks task 'build'", "sets `outputs`"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, stderr)
		}
	}
}
