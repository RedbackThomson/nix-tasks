//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const buildTasksConfig = `{
  tasks = {
    artifact = {
      type = "build";
      description = "Build a derivation unique to this test run";
      derivation = pkgs.runCommand "build-task-artifact" { outputs = [ "out" "doc" ]; } ''
        mkdir -p $out
        cp ${./marker} $out/marker
        echo docs > $doc
      '';
    };
    never-evaluated = {
      type = "build";
      description = "Listing must not instantiate build derivations";
      derivation = throw "build derivation evaluated during list";
    };
  };
}`

func TestRun_BuildTaskBuildsAndLinksOutputs(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	marker := randomString(t)
	dir := writeTempFlake(t, buildTasksConfig, map[string]string{"marker": marker})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "run", "artifact", "-f", dir)
	if code != 0 {
		t.Fatalf("run artifact exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".nix-tasks", "artifact", "out", "marker"))
	if err != nil {
		t.Fatalf("out link does not resolve to the built output: %v", err)
	}
	if strings.TrimSpace(string(got)) != marker {
		t.Errorf("marker = %q, want %q", got, marker)
	}
	if _, err := os.ReadFile(filepath.Join(dir, ".nix-tasks", "artifact", "doc")); err != nil {
		t.Errorf("doc output not linked by name: %v", err)
	}

	stdout, stderr, code = runNixTasksIn(t, binary, dir, "run", "artifact", "-f", dir)
	if code != 0 {
		t.Fatalf("second run exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "cached") {
		t.Errorf("build task must not be served from the task cache, got: %s", stdout)
	}
}

func TestList_DoesNotEvaluateBuildDerivations(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, buildTasksConfig, map[string]string{"marker": "x"})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code != 0 {
		t.Fatalf("list exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "never-evaluated") {
		t.Errorf("expected build task in list, got: %s", stdout)
	}
}
