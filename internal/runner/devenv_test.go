package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestRun_TaskShellEnvironment(t *testing.T) {
	h := newHarness(t)
	h.nix.drvs = map[string]string{"go": "/nix/store/go-nix-shell.drv"}
	tasks := map[string]config.Task{
		"plain": sh(`echo "plain $FAKE_SHELL" >> ran.log`),
		"go":    sh(`echo "go $FAKE_SHELL" >> ran.log`),
		"override": {
			Script: `echo "override $FAKE_SHELL" >> ran.log`,
			Env:    map[string]string{"FAKE_SHELL": "from-task-env"},
		},
		"all": {Depends: []string{"task:plain", "task:go", "task:override"}},
	}
	h.opts.Jobs = 1
	assertStates(t, h.run(context.Background(), tasks, "all"), map[string]runner.TaskState{
		"plain": runner.Succeeded, "go": runner.Succeeded, "override": runner.Succeeded, "all": runner.Succeeded,
	})

	data, err := os.ReadFile(filepath.Join(h.dir, "ran.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := "go /nix/store/go-nix-shell.drv\noverride from-task-env\nplain /nix/store/shared-nix-shell.drv\n"
	if string(data) != want {
		t.Errorf("ran.log = %q, want %q", data, want)
	}
}

func TestRun_TaskShellsRealisedOncePerDrv(t *testing.T) {
	h := newHarness(t)
	h.nix.drvs = map[string]string{"c": "/nix/store/other-nix-shell.drv"}
	tasks := map[string]config.Task{
		"a":   logged("a"),
		"b":   logged("b"),
		"c":   logged("c"),
		"all": {Depends: []string{"task:a", "task:b", "task:c"}},
	}

	h.run(context.Background(), tasks, "all")
	if len(h.nix.evals) != 1 || !slices.Equal(h.nix.evals[0], []string{"a", "b", "c"}) {
		t.Errorf("drvPaths should be evaluated once for the tasks that run a script, got %v", h.nix.evals)
	}
	realised := slices.Sorted(slices.Values(h.nix.realised))
	if !slices.Equal(realised, []string{"/nix/store/other-nix-shell.drv", "/nix/store/shared-nix-shell.drv"}) {
		t.Errorf("each shell should be realised once, got %v", h.nix.realised)
	}

	h.nix.realised = nil
	h.run(context.Background(), tasks, "all")
	if len(h.nix.realised) != 0 {
		t.Errorf("a second run should reuse the cached environments, realised %v", h.nix.realised)
	}
}

func TestRun_TaskShellRealisedAgainAfterGC(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{"a": logged("a")}
	h.run(context.Background(), tasks, "a")

	profiles, err := filepath.Glob(filepath.Join(h.opts.DevEnvDir, "*-profile"))
	if err != nil || len(profiles) != 1 {
		t.Fatalf("expected one profile link, got %v (%v)", profiles, err)
	}
	// A profile that no longer resolves is what garbage collection leaves.
	if err := os.Remove(profiles[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "collected"), profiles[0]); err != nil {
		t.Fatal(err)
	}

	h.nix.realised = nil
	assertStates(t, h.run(context.Background(), tasks, "a"), map[string]runner.TaskState{"a": runner.Succeeded})
	if len(h.nix.realised) != 1 {
		t.Errorf("a collected environment should be realised again, realised %v", h.nix.realised)
	}
}

func TestRun_TasksWithoutScriptSkipTheShell(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"a":     logged("a"),
		"group": {Depends: []string{"task:a"}},
		"empty": {Commands: []string{}},
	}
	assertStates(t, h.run(context.Background(), tasks, "group"), map[string]runner.TaskState{
		"a": runner.Succeeded, "group": runner.Succeeded,
	})
	assertStates(t, h.run(context.Background(), tasks, "empty"), map[string]runner.TaskState{
		"empty": runner.Succeeded,
	})
	for _, eval := range h.nix.evals {
		if slices.Contains(eval, "group") || slices.Contains(eval, "empty") {
			t.Errorf("evaluated the shell of a task with nothing to run: %v", h.nix.evals)
		}
	}
	if len(h.nix.realised) != 1 {
		t.Errorf("only a's shell should be realised, got %v", h.nix.realised)
	}
}

func TestRun_BrokenShellFailsOnlyItsTask(t *testing.T) {
	h := newHarness(t)
	h.nix.broken = map[string]bool{"bad": true}
	tasks := map[string]config.Task{
		"good": logged("good"),
		"bad":  logged("bad"),
		"all":  {Depends: []string{"task:good", "task:bad"}},
	}
	h.opts.ContinueOnError = true
	results := h.run(context.Background(), tasks, "all")
	assertStates(t, results, map[string]runner.TaskState{
		"good": runner.Succeeded, "bad": runner.Failed, "all": runner.Skipped,
	})
	if err := results["bad"].Error; err == nil || !strings.Contains(err.Error(), "shell of bad is broken") {
		t.Errorf("bad should fail with its eval error, got %v", err)
	}
}

func TestRun_PrecomputedShellDrvPathSkipsEval(t *testing.T) {
	h := newHarness(t)
	task := logged("a")
	task.ShellDrvPath = "/nix/store/known-nix-shell.drv"
	h.run(context.Background(), map[string]config.Task{"a": task}, "a")
	if len(h.nix.evals) != 0 {
		t.Errorf("a task with a known shellDrvPath should not be evaluated, got %v", h.nix.evals)
	}
	if !slices.Equal(h.nix.realised, []string{"/nix/store/known-nix-shell.drv"}) {
		t.Errorf("realised %v", h.nix.realised)
	}
}
