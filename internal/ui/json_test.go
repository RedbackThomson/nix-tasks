package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// steppingClock returns start, then start plus step for each later call.
func steppingClock(start time.Time, step time.Duration) func() time.Time {
	now := start.Add(-step)
	return func() time.Time {
		now = now.Add(step)
		return now
	}
}

// taskExit wraps a process exit with code the way the runner reports it.
func taskExit(t *testing.T, name string, code int) error {
	t.Helper()
	return &runner.TaskExecutionError{Name: name, Err: exitError(t, code)}
}

func jsonScenario(t *testing.T) (*runner.TaskGraph, runner.RunPlan, []runner.TaskResult) {
	t.Helper()
	tasks := map[string]config.Task{
		"gen":   {},
		"build": {Depends: []string{"gen"}},
		// Both a dependency of deploy and a hook of build, so it is not
		// reported as a hook: it would have run anyway.
		"lint":   {ContinueOnError: true, After: []string{"build"}},
		"test":   {Depends: []string{"build"}},
		"deploy": {Depends: []string{"test", "lint"}},
		"notify": {After: []string{"build"}, Depends: []string{"setup"}},
		// Needed only by the hook notify: a dependency, not a hook.
		"setup": {},
	}
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 20, 9, 15, 2, 0, time.UTC)
	at := func(ms int) time.Time { return start.Add(time.Duration(ms) * time.Millisecond) }
	results := []runner.TaskResult{
		{Name: "gen", State: runner.Succeeded, Cached: true, Start: at(100), Duration: time.Millisecond},
		{Name: "lint", State: runner.FailedAllowed, Error: taskExit(t, "lint", 1), Start: at(100), Duration: 400 * time.Millisecond, Log: "/runs/r/lint.log"},
		{Name: "build", State: runner.Succeeded, Start: at(101), Duration: 2300 * time.Millisecond, Log: "/runs/r/build.log"},
		{Name: "setup", State: runner.Succeeded, Start: at(2401), Duration: 10 * time.Millisecond, Log: "/runs/r/setup.log"},
		{Name: "notify", State: runner.Succeeded, Start: at(2411), Duration: 50 * time.Millisecond, Log: "/runs/r/notify.log"},
		{Name: "test", State: runner.Failed, Error: taskExit(t, "test", 2), Start: at(2401), Duration: 4100 * time.Millisecond, Log: "/runs/r/test.log"},
		{Name: "deploy", State: runner.Skipped, SkipReason: "test failed"},
	}
	plan := runner.RunPlan{
		Targets: []string{"deploy"},
		Tasks:   []string{"gen", "build", "lint", "setup", "notify", "test", "deploy"},
		Labels:  map[string]string{"notify": "ntfy"},
	}
	return graph, plan, results
}

func TestJSONReporter_Golden(t *testing.T) {
	graph, plan, results := jsonScenario(t)
	var out bytes.Buffer
	r := NewJSONReporter(JSONOptions{
		Out:     &out,
		Targets: []string{"deploy"},
		Flake:   "/repo",
		System:  "aarch64-darwin",
		Now:     steppingClock(time.Date(2026, 11, 20, 9, 15, 0, 0, time.UTC), 7*time.Second),
	})
	r.RunPlanned(plan)
	r.Start()
	for _, res := range results {
		r.TaskFinished(res)
	}
	r.Summary(graph, plan.Targets, results)
	r.Stop()
	if err := r.Write(2, []string{"task logs are not kept: example"}, nil); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "json_run.golden.json", out.String())
}

func TestJSONReporter_RunError(t *testing.T) {
	var out bytes.Buffer
	r := NewJSONReporter(JSONOptions{Out: &out, Targets: []string{"nope"}, Flake: ".", System: "x86_64-linux",
		Now: steppingClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)})
	if err := r.Write(1, nil, errors.New("task not found: nope")); err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if doc["error"] != "task not found: nope" || doc["exitCode"] != float64(1) {
		t.Errorf("unexpected document:\n%s", out.String())
	}
	for _, field := range []string{"tasks", "warnings"} {
		if list, ok := doc[field].([]any); !ok || len(list) != 0 {
			t.Errorf("%s should be an empty list, got %v", field, doc[field])
		}
	}
	if doc["durationMs"] != float64(1000) {
		t.Errorf("durationMs = %v", doc["durationMs"])
	}
}

func TestJSONReporter_EveryTaskFieldPresent(t *testing.T) {
	graph, plan, results := jsonScenario(t)
	var out bytes.Buffer
	r := NewJSONReporter(JSONOptions{Out: &out})
	r.RunPlanned(plan)
	r.Summary(graph, plan.Targets, results)
	if err := r.Write(0, nil, nil); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	fields := []string{"name", "label", "state", "cached", "exitCode", "error", "skipReason",
		"startedAt", "finishedAt", "durationMs", "afterHook", "log"}
	for _, task := range doc.Tasks {
		for _, f := range fields {
			if _, ok := task[f]; !ok {
				t.Errorf("task %v has no %q field", task["name"], f)
			}
		}
		if len(task) != len(fields) {
			t.Errorf("task %v has fields beyond the schema: %v", task["name"], task)
		}
	}
}

func TestJSONReporter_DiscardsTaskOutput(t *testing.T) {
	r := NewJSONReporter(JSONOptions{Out: &bytes.Buffer{}})
	stdout, stderr := r.TaskOutput("t")
	if stdout != stderr {
		t.Error("stdout and stderr should be one writer, so the task's log stays in order")
	}
}

func TestJSONTime(t *testing.T) {
	ts := JSONTime(time.Date(2026, 1, 2, 3, 4, 5, 100_000_000, time.FixedZone("x", 3600)))
	data, err := json.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"2026-01-02T02:04:05.100Z"` {
		t.Errorf("marshalled %s", data)
	}
	var back JSONTime
	if err := json.Unmarshal(data, &back); err != nil || !time.Time(back).Equal(time.Time(ts)) {
		t.Errorf("round trip gave %v (%v)", time.Time(back), err)
	}
}

func TestJSONReporter_AfterHook(t *testing.T) {
	graph, plan, results := jsonScenario(t)
	var out bytes.Buffer
	r := NewJSONReporter(JSONOptions{Out: &out})
	r.RunPlanned(plan)
	r.Summary(graph, plan.Targets, results)
	if err := r.Write(0, nil, nil); err != nil {
		t.Fatal(err)
	}
	var doc JSONDocument
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, task := range doc.Tasks {
		if want := task.Name == "notify"; task.AfterHook != want {
			t.Errorf("%s: afterHook %v, want %v", task.Name, task.AfterHook, want)
		}
	}
}
