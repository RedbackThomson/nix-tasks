//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

const builderTasksConfig = `{
  packages = { docker = pkgs.hello; kubectl = pkgs.hello; };
  tasks = {
    a = tasksLib.mkTask { commands = [ "true" ]; };
    b = tasksLib.mkTask { commands = [ "true" ]; };
    image = tasksLib.mkDockerTask { image = "app"; deps = [ "kubectl" ]; };
    group = tasksLib.mkCompoundTask { tasks = [ "a" ]; depends = [ "task:b" ]; };
    gobuild = tasksLib.mkGoTask {
      pname = "app";
      src = ./.;
      vendorHash = null;
      env = { EXTRA_FLAG = "set"; };
      continueOnError = true;
    };
  };
}`

// evalTaskConfig evaluates nixTasksConfig.<system>.tasks directly, testing
// the builders' output independent of the CLI.
func evalTaskConfig(t *testing.T, dir string) map[string]map[string]any {
	t.Helper()
	attr := fmt.Sprintf("%s#nixTasksConfig.%s.tasks", dir, nix.QuoteAttr(nix.CurrentSystem()))
	out, err := exec.Command("nix", "eval", "--json", attr).Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("nix eval %s failed: %v\n%s", attr, err, stderr)
	}
	var tasks map[string]map[string]any
	if err := json.Unmarshal(out, &tasks); err != nil {
		t.Fatal(err)
	}
	return tasks
}

func asStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func TestBuilders_KeepUserArguments(t *testing.T) {
	skipIfNoNix(t)

	dir := writeTempFlake(t, builderTasksConfig, map[string]string{})
	tasks := evalTaskConfig(t, dir)

	if deps := asStrings(tasks["image"]["deps"]); !slices.Contains(deps, "docker") || !slices.Contains(deps, "kubectl") {
		t.Errorf("mkDockerTask deps = %v, want docker and kubectl", deps)
	}

	if depends := asStrings(tasks["group"]["depends"]); !slices.Contains(depends, "task:a") || !slices.Contains(depends, "task:b") {
		t.Errorf("mkCompoundTask depends = %v, want task:a and task:b", depends)
	}

	gobuild := tasks["gobuild"]
	if gobuild["continueOnError"] != true {
		t.Errorf("mkGoTask continueOnError = %v, want true", gobuild["continueOnError"])
	}

	for name, want := range map[string]string{"CGO_ENABLED": "0", "EXTRA_FLAG": "set"} {
		attr := fmt.Sprintf("%s#nixTasksBuilds.%s.gobuild.%s", dir, nix.QuoteAttr(nix.CurrentSystem()), name)
		out, err := exec.Command("nix", "eval", "--raw", attr).Output()
		if err != nil || string(out) != want {
			t.Errorf("derivation %s = %q (err %v), want %s", name, out, err, want)
		}
	}
}

func TestBuilders_MkGoTaskRequiresSrc(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.gobuild = tasksLib.mkGoTask { pname = "app"; vendorHash = null; };
}`, map[string]string{})

	_, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code == 0 {
		t.Fatal("expected list to fail when mkGoTask has no src")
	}
	if !strings.Contains(stderr, "src") {
		t.Errorf("expected error to mention src, got: %s", stderr)
	}
}
