package runner_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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

// recorder captures each task's output and the order of reporter events.
type recorder struct {
	mu       sync.Mutex
	outputs  map[string]*bytes.Buffer
	events   []string
	finished map[string]int
}

func newRecorder() *recorder {
	return &recorder{outputs: map[string]*bytes.Buffer{}, finished: map[string]int{}}
}

func (r *recorder) TaskStarted(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "start "+name)
}

func (r *recorder) TaskOutput(name string) (io.Writer, io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.outputs[name] == nil {
		r.outputs[name] = &bytes.Buffer{}
	}
	w := &lockedWriter{mu: &r.mu, buf: r.outputs[name]}
	return w, w
}

func (r *recorder) TaskFinished(res runner.TaskResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "finish "+res.Name)
	r.finished[res.Name]++
}

func (r *recorder) output(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.outputs[name]; b != nil {
		return b.String()
	}
	return ""
}

type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

type harness struct {
	t    *testing.T
	dir  string
	opts runner.Options
	rec  *recorder
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{t: t, dir: t.TempDir(), rec: newRecorder()}
}

func (h *harness) run(ctx context.Context, tasks map[string]config.Task, target string) map[string]runner.TaskResult {
	h.t.Helper()
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		h.t.Fatalf("NewTaskGraph: %v", err)
	}
	h.rec = newRecorder()
	opts := h.opts
	opts.FlakeRoot = h.dir
	opts.Reporter = h.rec
	ex := runner.NewExecutor(fakeNix{}, &config.Config{Tasks: tasks}, opts)
	results, err := ex.Run(ctx, graph, target)
	if err != nil {
		h.t.Fatalf("Run: %v", err)
	}
	byName := make(map[string]runner.TaskResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
		if n := h.rec.finished[r.Name]; n != 1 {
			h.t.Errorf("%s: reported finished %d times, want 1", r.Name, n)
		}
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
			t.Errorf("%s: state %v, want %v (err: %v)", name, r.State, state, r.Error)
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
	if out := h.rec.output("bad"); !strings.Contains(out, "boom") {
		t.Errorf("failed output not reported: %q", out)
	}
	if code := results["bad"].ExitCode(); code != 3 {
		t.Errorf("ExitCode() = %d, want 3", code)
	}
	var exitErr *exec.ExitError
	if r := results["bad"]; r.Error == nil || !errors.As(r.Error, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("expected exit status 3 in error, got %v", r.Error)
	}
}

func TestRun_ReportsEvents(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"bad":   sh("echo out; echo err >&2; exit 1"),
		"after": logged("after", "bad"),
	}
	results := h.run(context.Background(), tasks, "after")

	want := []string{"start bad", "finish bad", "finish after"}
	if !slices.Equal(h.rec.events, want) {
		t.Errorf("events = %v, want %v", h.rec.events, want)
	}
	if out := h.rec.output("bad"); out != "out\nerr\n" {
		t.Errorf("output = %q, want stdout and stderr", out)
	}
	if r := results["bad"]; r.Start.IsZero() || r.Duration <= 0 {
		t.Errorf("started task should have a start time and duration: %+v", r)
	}
	if r := results["after"]; !r.Start.IsZero() || r.ExitCode() != -1 {
		t.Errorf("skipped task should have no start time or exit code: %+v", r)
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
	if out := h.rec.output("flaky"); !strings.Contains(out, "flaky-output") {
		t.Errorf("allowed failure output not reported: %q", out)
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
	if !slices.Equal(h.rec.events, []string{"finish ok"}) {
		t.Errorf("a cache hit should be reported finished without starting, got %v", h.rec.events)
	}

	h.run(context.Background(), tasks, "bad")
	if r := h.run(context.Background(), tasks, "bad")["bad"]; r.State != runner.Failed || r.Cached {
		t.Errorf("failures must not be cached: %+v", r)
	}
	if ran := h.ran(); !slices.Equal(ran, []string{"ok", "bad", "bad"}) {
		t.Errorf("ran %v", ran)
	}
}

// assertProcessGone fails if the process whose pid the task wrote to file is
// still alive.
func assertProcessGone(t *testing.T, file string) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("task did not record its child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("child process %d outlived its task", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRun_FailFastInterruptsRunningTasks(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"bad":  sh("sleep 0.3; exit 1"),
		"slow": sh("sleep 30 & echo $! > slow.pid; wait; echo slow >> ran.log"),
		"all":  logged("all", "bad", "slow"),
	}
	start := time.Now()
	results := h.run(context.Background(), tasks, "all")
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("fail-fast took %v; the running sibling should be stopped at once", elapsed)
	}

	assertStates(t, results, map[string]runner.TaskState{
		"bad": runner.Failed, "slow": runner.Interrupted, "all": runner.Skipped,
	})
	if ran := h.ran(); len(ran) != 0 {
		t.Errorf("ran %v", ran)
	}
	assertProcessGone(t, filepath.Join(h.dir, "slow.pid"))
}

func TestRun_Interrupted(t *testing.T) {
	h := newHarness(t)
	tasks := map[string]config.Task{
		"slow":  sh("sleep 30 & echo $! > slow.pid; wait"),
		"after": logged("after", "slow"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	start := time.Now()
	results := h.run(ctx, tasks, "after")
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("interrupted run took %v", elapsed)
	}

	assertStates(t, results, map[string]runner.TaskState{"slow": runner.Interrupted, "after": runner.Skipped})
	assertProcessGone(t, filepath.Join(h.dir, "slow.pid"))
}

func TestRun_CancelledBeforeStartSkipsEverything(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := h.run(ctx, map[string]config.Task{"a": logged("a"), "b": logged("b", "a")}, "b")
	assertStates(t, results, map[string]runner.TaskState{"a": runner.Skipped, "b": runner.Skipped})
}

func TestRun_ShellOptions(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   runner.TaskState
	}{
		{"failing command stops the script", "false\necho unreachable >> ran.log", runner.Failed},
		{"failure inside a pipeline fails the task", "false | cat", runner.Failed},
		{"pipefail can be turned off", "set +o pipefail\nfalse | cat", runner.Succeeded},
		{"a failure can be tolerated with || true", "false || true", runner.Succeeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			results := h.run(context.Background(), map[string]config.Task{"t": sh(tt.script)}, "t")
			assertStates(t, results, map[string]runner.TaskState{"t": tt.want})
			if ran := h.ran(); len(ran) != 0 {
				t.Errorf("script continued after a failure: %v", ran)
			}
		})
	}
}
