//go:build integration

package integration_test

import (
	"testing"
)

func TestRun_PipelineFailureFailsTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, `{
  tasks.piped = tasksLib.mkTask { commands = [ "exit 7 | cat" ]; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "run", "piped", "-f", dir)
	if code != 7 {
		t.Errorf("exited %d, want 7 (pipefail)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}
