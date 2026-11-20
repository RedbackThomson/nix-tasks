package runner_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// fakeNix runs task scripts with the host bash instead of inside nix develop.
type fakeNix struct{}

func (fakeNix) DevelopCmd(ctx context.Context, _ string, command []string) *exec.Cmd {
	return exec.CommandContext(ctx, command[0], command[1:]...)
}

func (fakeNix) BuildCmd(ctx context.Context, _ string) *exec.Cmd {
	return exec.CommandContext(ctx, "false")
}

// sh returns a task that runs script; scripts run in the test's temp dir.
func sh(script string, depends ...string) config.Task {
	var deps []string
	for _, d := range depends {
		deps = append(deps, config.TaskDependencyPrefix+d)
	}
	return config.Task{Script: script, Depends: deps}
}

// logged returns a task that appends its name to ran.log.
func logged(name string, depends ...string) config.Task {
	return sh("echo "+name+" >> ran.log", depends...)
}

type harness struct {
	t    *testing.T
	dir  string
	opts runner.Options
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{t: t, dir: t.TempDir()}
}

func (h *harness) run(ctx context.Context, tasks map[string]config.Task, target string) map[string]runner.TaskResult {
	h.t.Helper()
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		h.t.Fatalf("NewTaskGraph: %v", err)
	}
	opts := h.opts
	opts.FlakeRoot = h.dir
	ex := runner.NewExecutor(fakeNix{}, &config.Config{Tasks: tasks}, opts)
	results, err := ex.Run(ctx, graph, target)
	if err != nil {
		h.t.Fatalf("Run: %v", err)
	}
	byName := make(map[string]runner.TaskResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}
	return byName
}

func (h *harness) ran() []string {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, "ran.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func assertStates(t *testing.T, results map[string]runner.TaskResult, want map[string]runner.TaskState) {
	t.Helper()
	if len(results) != len(want) {
		t.Errorf("got %d results, want %d: %v", len(results), len(want), results)
	}
	for name, state := range want {
		r, ok := results[name]
		if !ok {
			t.Errorf("%s: no result", name)
			continue
		}
		if r.State != state {
			t.Errorf("%s: state %v, want %v (err: %v, output: %q)", name, r.State, state, r.Error, r.Output)
		}
	}
}

func TestRun_DependencyOrder(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"root":  logged("root"),
		"left":  logged("left", "root"),
		"right": logged("right", "root"),
		"top":   logged("top", "left", "right"),
	}
	results := h.run(context.Background(), tasks, "top")

	assertStates(t, results, map[string]runner.TaskState{
		"root": runner.Succeeded, "left": runner.Succeeded, "right": runner.Succeeded, "top": runner.Succeeded,
	})
	ran := h.ran()
	if len(ran) != 4 || ran[0] != "root" || ran[3] != "top" {
		t.Errorf("unexpected order %v", ran)
	}
}

func TestRun_ResultsInDeterministicOrder(t *testing.T) {
	tasks := map[string]config.Task{
		"c": logged("c"), "a": logged("a"), "b": logged("b"),
		"all": logged("all", "c", "a", "b"),
	}
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		ex := runner.NewExecutor(fakeNix{}, &config.Config{Tasks: tasks}, runner.Options{FlakeRoot: t.TempDir()})
		results, err := ex.Run(context.Background(), graph, "all")
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range results {
			names = append(names, r.Name)
		}
		if want := []string{"c", "a", "b", "all"}; !slices.Equal(names, want) {
			t.Fatalf("result order %v, want %v", names, want)
		}
	}
}

func TestRun_FailedDependencySkipsDependents(t *testing.T) {
	h := newHarness(t)
	h.opts.ContinueOnError = true
	tasks := map[string]config.Task{
		"bad":        sh("echo boom; exit 3"),
		"good":       logged("good"),
		"needs-bad":  logged("needs-bad", "bad"),
		"transitive": logged("transitive", "needs-bad"),
		"all":        logged("all", "good", "transitive"),
	}
	results := h.run(context.Background(), tasks, "all")

	assertStates(t, results, map[string]runner.TaskState{
		"bad": runner.Failed, "good": runner.Succeeded,
		"needs-bad": runner.Skipped, "transitive": runner.Skipped, "all": runner.Skipped,
	})
	if ran := h.ran(); !slices.Equal(ran, []string{"good"}) {
		t.Errorf("only good should run, ran %v", ran)
	}
	if !strings.Contains(results["bad"].Output, "boom") {
		t.Errorf("failed output not kept: %q", results["bad"].Output)
	}
	var exitErr *exec.ExitError
	if r := results["bad"]; r.Error == nil || !errors.As(r.Error, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("expected exit status 3 in error, got %v", r.Error)
	}
}

func TestRun_TaskContinueOnError(t *testing.T) {
	h := newHarness(t)
	flaky := sh("echo flaky-output; exit 1")
	flaky.ContinueOnError = true
	tasks := map[string]config.Task{
		"flaky": flaky,
		"after": logged("after", "flaky"),
	}
	results := h.run(context.Background(), tasks, "after")

	assertStates(t, results, map[string]runner.TaskState{"flaky": runner.FailedAllowed, "after": runner.Succeeded})
	if !strings.Contains(results["flaky"].Output, "flaky-output") {
		t.Errorf("allowed failure output not kept: %q", results["flaky"].Output)
	}
	if results["flaky"].Error == nil {
		t.Error("allowed failure should keep its error")
	}
}

func TestRun_FailFastStopsUnstartedTasks(t *testing.T) {
	h := newHarness(t)
	h.opts.Jobs = 1
	tasks := map[string]config.Task{
		"a-fails": sh("exit 1"),
		"b":       logged("b"),
		"c":       logged("c"),
		"all":     logged("all", "a-fails", "b", "c"),
	}
	results := h.run(context.Background(), tasks, "all")

	assertStates(t, results, map[string]runner.TaskState{
		"a-fails": runner.Failed, "b": runner.Skipped, "c": runner.Skipped, "all": runner.Skipped,
	})
	if ran := h.ran(); len(ran) != 0 {
		t.Errorf("nothing should run after the failure, ran %v", ran)
	}
}

func TestRun_AfterHooks(t *testing.T) {
	hook := logged("hook")
	hook.After = []string{config.TaskDependencyPrefix + "build"}

	t.Run("hook runs after its target", func(t *testing.T) {
		h := newHarness(t)
		results := h.run(context.Background(), map[string]config.Task{"build": logged("build"), "hook": hook}, "build")
		assertStates(t, results, map[string]runner.TaskState{"build": runner.Succeeded, "hook": runner.Succeeded})
		if ran := h.ran(); !slices.Equal(ran, []string{"build", "hook"}) {
			t.Errorf("ran %v", ran)
		}
	})

	t.Run("hook is skipped when its target fails", func(t *testing.T) {
		h := newHarness(t)
		h.opts.ContinueOnError = true
		results := h.run(context.Background(), map[string]config.Task{"build": sh("exit 1"), "hook": hook}, "build")
		assertStates(t, results, map[string]runner.TaskState{"build": runner.Failed, "hook": runner.Skipped})
	})

	t.Run("hook does not run for an unrelated target", func(t *testing.T) {
		h := newHarness(t)
		results := h.run(context.Background(), map[string]config.Task{
			"build": logged("build"), "hook": hook, "other": logged("other"),
		}, "other")
		assertStates(t, results, map[string]runner.TaskState{"other": runner.Succeeded})
	})
}

func TestRun_JobsBound(t *testing.T) {
	h := newHarness(t)
	h.opts.Jobs = 2
	if err := os.Mkdir(filepath.Join(h.dir, "running"), 0o755); err != nil {
		t.Fatal(err)
	}
	busy := func(name string) config.Task {
		return sh("touch running/" + name + "; ls running | wc -l >> counts; sleep 0.3; rm running/" + name)
	}
	tasks := map[string]config.Task{
		"a": busy("a"), "b": busy("b"), "c": busy("c"), "d": busy("d"), "e": busy("e"),
		"all": sh("true", "a", "b", "c", "d", "e"),
	}
	results := h.run(context.Background(), tasks, "all")
	if results["all"].State != runner.Succeeded {
		t.Fatalf("run failed: %v", results)
	}

	data, err := os.ReadFile(filepath.Join(h.dir, "counts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range strings.Fields(string(data)) {
		if n, _ := strconv.Atoi(f); n > 2 {
			t.Errorf("%d tasks ran at once with -j 2", n)
		}
	}
}

func TestRun_StartsTasksWithoutWaitingForUnrelatedOnes(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"slow":  sh("sleep 1; echo slow >> ran.log"),
		"fast":  logged("fast"),
		"after": logged("after", "fast"),
		"all":   sh("true", "slow", "after"),
	}
	h.run(context.Background(), tasks, "all")
	if ran := h.ran(); !slices.Equal(ran, []string{"fast", "after", "slow"}) {
		t.Errorf("after should not wait for slow, ran %v", ran)
	}
}

func TestRun_Cache(t *testing.T) {
	t.Setenv("NIX_TASKS_CACHE_DIR", t.TempDir())
	h := newHarness(t)
	h.opts.ProjectKey = "test"
	if err := os.WriteFile(filepath.Join(h.dir, "input.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cachedTask := func(script string) config.Task {
		task := sh(script)
		task.Inputs = []string{"input.txt"}
		return task
	}
	tasks := map[string]config.Task{
		"ok":  cachedTask("echo ok >> ran.log"),
		"bad": cachedTask("echo bad >> ran.log; exit 1"),
	}

	first := h.run(context.Background(), tasks, "ok")
	if r := first["ok"]; r.State != runner.Succeeded || r.Cached {
		t.Fatalf("first run: %+v", r)
	}
	second := h.run(context.Background(), tasks, "ok")
	if r := second["ok"]; r.State != runner.Succeeded || !r.Cached {
		t.Fatalf("second run should be a cache hit: %+v", r)
	}

	h.run(context.Background(), tasks, "bad")
	if r := h.run(context.Background(), tasks, "bad")["bad"]; r.State != runner.Failed || r.Cached {
		t.Errorf("failures must not be cached: %+v", r)
	}
	if ran := h.ran(); !slices.Equal(ran, []string{"ok", "bad", "bad"}) {
		t.Errorf("ran %v", ran)
	}
}
