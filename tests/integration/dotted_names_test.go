//go:build integration

package integration_test

import (
	"strings"
	"testing"
)

func TestRun_TaskNameWithDot(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	stdout, stderr, err := runNixTasks(t, binary, "run", "lint.go", "--stream", "-f", namesAndExitCodesFlake(t))
	if err != nil {
		t.Fatalf("run lint.go failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "linted go") {
		t.Errorf("expected task output, got: %s", stdout)
	}
}

func TestShell_NameWithDot(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	stdout, stderr, err := runNixTasks(t, binary, "shell", "ci.full", "-c", "echo name=$SHELL_NAME", "-f", namesAndExitCodesFlake(t))
	if err != nil {
		t.Fatalf("shell ci.full failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "name=ci.full") {
		t.Errorf("expected shell env in output, got: %s", stdout)
	}
}
