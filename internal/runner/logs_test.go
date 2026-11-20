package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRun_TaskLogs(t *testing.T) {
	h := newHarness(t)
	logDir := t.TempDir()
	h.opts.LogDir = logDir
	tasks := map[string]config.Task{
		"build":  sh("echo out; echo err >&2"),
		"fail":   {Script: "echo broken; exit 3", ContinueOnError: true},
		"after":  sh("echo after", "fail"),
		"group":  {Depends: []string{"build", "after"}},
		"ns/sub": sh("echo nested"),
	}

	results := h.run(context.Background(), tasks, "group")
	assertStates(t, results, map[string]runner.TaskState{
		"build": runner.Succeeded, "fail": runner.FailedAllowed, "after": runner.Succeeded, "group": runner.Succeeded,
	})
	for name, want := range map[string]string{
		"build": "out\nerr\n",
		"fail":  "broken\n",
		"after": "after\n",
		"group": "",
	} {
		r := results[name]
		if r.Log != filepath.Join(logDir, name+".log") {
			t.Errorf("%s: log %q, want it in %s", name, r.Log, logDir)
			continue
		}
		if got := readLog(t, r.Log); got != want {
			t.Errorf("%s: log holds %q, want %q", name, got, want)
		}
	}

	nested := h.run(context.Background(), tasks, "ns/sub")["ns/sub"]
	if nested.Log != filepath.Join(logDir, "ns_sub.log") || readLog(t, nested.Log) != "nested\n" {
		t.Errorf("a task name with / should log to ns_sub.log, got %q", nested.Log)
	}
}

func TestRun_NoLogForTasksThatDidNotRun(t *testing.T) {
	h := newHarness(t)
	h.opts.LogDir = t.TempDir()
	h.opts.Cache = cache.Open(t.TempDir(), "test")
	if err := os.WriteFile(filepath.Join(h.dir, "input.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cached := sh("echo cached")
	cached.Cache = &config.TaskCache{Inputs: []string{"input.txt"}, Outputs: []string{}}
	tasks := map[string]config.Task{
		"cached": cached,
		"bad":    sh("exit 1"),
		"dep":    sh("echo dep", "bad"),
	}

	h.run(context.Background(), tasks, "cached")
	if r := h.run(context.Background(), tasks, "cached")["cached"]; !r.Cached || r.Log != "" {
		t.Errorf("a cache hit should have no log: %+v", r)
	}
	if r := h.run(context.Background(), tasks, "dep")["dep"]; r.State != runner.Skipped || r.Log != "" {
		t.Errorf("a skipped task should have no log: %+v", r)
	}
}

func TestRun_UnwritableLogDirWarns(t *testing.T) {
	h := newHarness(t)
	h.opts.LogDir = filepath.Join(t.TempDir(), "missing")
	var warnings []string
	h.opts.Warn = func(msg string) { warnings = append(warnings, msg) }

	r := h.run(context.Background(), map[string]config.Task{"a": logged("a")}, "a")["a"]
	if r.State != runner.Succeeded || r.Log != "" {
		t.Errorf("the task should run without a log: %+v", r)
	}
	if len(warnings) != 1 {
		t.Errorf("expected one warning, got %v", warnings)
	}
}

func TestRun_NoLogDirWritesNothing(t *testing.T) {
	h := newHarness(t)
	if r := h.run(context.Background(), map[string]config.Task{"a": logged("a")}, "a")["a"]; r.Log != "" {
		t.Errorf("no LogDir should mean no log, got %q", r.Log)
	}
}
