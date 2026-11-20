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
  tasks.build = tasksLib.mkTask {
    commands = [ "true" ];
    outputs = [ "bin/app" ];
  };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list")
	if code == 0 {
		t.Fatalf("expected list to fail, got success\nstdout: %s", stdout)
	}
	for _, want := range []string{"task 'build'", "cache.outputs"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, stderr)
		}
	}
}

func TestConfig_ConflictingCommandsExplainFix(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `lib.mkMerge [
  { tasks.test.commands = [ "go test ./..." ]; }
  { tasks.test.commands = [ "go test -race ./..." ]; }
]`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code == 0 {
		t.Fatalf("expected list to fail, got stdout: %s", stdout)
	}
	for _, want := range []string{"tasks.test.commands", "lib.mkForce", "lib.mkBefore"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, stderr)
		}
	}
}

func TestConfig_AssertionsReportEveryProblem(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.a = { deps = [ "missing-pkg" ]; depends = [ "task:a" "task:missing" ]; commands = [ "true" ]; };
  devShells = { x.extends = "y"; y.extends = "x"; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code == 0 || code == 139 {
		t.Fatalf("expected an evaluation error exit, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{
		"unknown package(s) in deps: missing-pkg",
		"depends on unknown task(s) task:missing",
		"depends on or runs after itself",
		"circular inheritance x -> y -> x",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, stderr)
		}
	}
}
