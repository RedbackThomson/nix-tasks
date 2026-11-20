//go:build integration

package integration_test

import (
	"strings"
	"testing"
)

func TestShell_DebugKeepsStdoutClean(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := namesAndExitCodesFlake(t)

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "--debug", "shell", "-c", "echo only-this", "-f", dir)
	if code != 0 {
		t.Fatalf("exited %d\nstderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "only-this" {
		t.Errorf("stdout should contain only the command output, got: %q", stdout)
	}
}
