package ui

import (
	"encoding/json"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// JSONVersion is the version of the --output=json document. It changes only
// when a field is removed or its meaning changes; new fields may be added
// without bumping it.
const JSONVersion = 1

// JSONDocument is what --output=json prints when a run finishes. Every field
// is always present, null when it does not apply, so jq queries need no
// existence checks.
type JSONDocument struct {
	Version    int        `json:"version"`
	Targets    []string   `json:"targets"`
	Flake      string     `json:"flake"`
	System     string     `json:"system"`
	StartedAt  JSONTime   `json:"startedAt"`
	FinishedAt JSONTime   `json:"finishedAt"`
	DurationMs int64      `json:"durationMs"`
	ExitCode   int        `json:"exitCode"`
	Error      *string    `json:"error"`
	Counts     JSONCounts `json:"counts"`
	Warnings   []string   `json:"warnings"`
	Tasks      []JSONTask `json:"tasks"`
}

// JSONCounts totals the run's tasks by outcome. Cache hits are counted as
// cached, not passed.
type JSONCounts struct {
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	AllowedFailures int `json:"allowedFailures"`
	Cached          int `json:"cached"`
	Skipped         int `json:"skipped"`
	Interrupted     int `json:"interrupted"`
}

// JSONTask is one task of the run.
type JSONTask struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Cached bool   `json:"cached"`
	// AfterHook is set when the task is in the run because it is an
	// after-hook of another task in it; a task the targets also depend on,
	// and a dependency of a hook, is not.
	AfterHook  bool      `json:"afterHook"`
	ExitCode   *int      `json:"exitCode"`
	Error      *string   `json:"error"`
	SkipReason *string   `json:"skipReason"`
	StartedAt  *JSONTime `json:"startedAt"`
	FinishedAt *JSONTime `json:"finishedAt"`
	DurationMs *int64    `json:"durationMs"`
	Log        *string   `json:"log"`
}

// JSONTime is an RFC 3339 UTC timestamp with exactly three fractional
// digits, so timestamps have one width and compare as strings.
type JSONTime time.Time

const jsonTimeLayout = "2006-01-02T15:04:05.000Z07:00"

func (t JSONTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(jsonTimeLayout))
}

func (t *JSONTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	*t = JSONTime(parsed)
	return err
}

// JSONOptions configures a JSONReporter.
type JSONOptions struct {
	Out     io.Writer
	Targets []string
	Flake   string
	System  string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

// JSONReporter prints nothing while tasks run and one JSONDocument when
// Write is called. Task output is never part of the document; it is in each
// task's log.
type JSONReporter struct {
	opts    JSONOptions
	started time.Time

	mu    sync.Mutex
	plan  runner.RunPlan
	tasks []JSONTask
}

// NewJSONReporter creates a JSONReporter; the run's start time is taken now.
func NewJSONReporter(opts JSONOptions) *JSONReporter {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &JSONReporter{opts: opts, started: opts.Now()}
}

func (r *JSONReporter) Start()             {}
func (r *JSONReporter) Interrupt()         {}
func (r *JSONReporter) Stop()              {}
func (r *JSONReporter) TaskStarted(string) {}

func (r *JSONReporter) TaskOutput(string) (io.Writer, io.Writer) {
	return io.Discard, io.Discard
}

func (r *JSONReporter) TaskFinished(runner.TaskResult) {}

func (r *JSONReporter) RunPlanned(plan runner.RunPlan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plan = plan
}

// Summary records the results in the plan's dependency order: every task
// after the tasks it depends on, ties broken by name.
func (r *JSONReporter) Summary(graph *runner.TaskGraph, targets []string, results []runner.TaskResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	byName := make(map[string]runner.TaskResult, len(results))
	for _, res := range results {
		byName[res.Name] = res
	}
	order := r.plan.Tasks
	if len(order) != len(results) {
		order = make([]string, 0, len(results))
		for _, res := range results {
			order = append(order, res.Name)
		}
	}
	needed := dependencyClosure(graph, targets)
	r.tasks = make([]JSONTask, 0, len(order))
	for _, name := range order {
		t := jsonTask(byName[name], labelOf(r.plan.Labels, name))
		t.AfterHook = !needed[name] && hooksInto(graph, name, byName)
		r.tasks = append(r.tasks, t)
	}
}

// hooksInto reports whether name runs after a task in the run.
func hooksInto(graph *runner.TaskGraph, name string, run map[string]runner.TaskResult) bool {
	task, ok := graph.Task(name)
	if !ok {
		return false
	}
	return slices.ContainsFunc(task.After, func(followed string) bool {
		_, in := run[followed]
		return in
	})
}

// dependencyClosure is the targets and everything they depend on, without
// following after-hooks.
func dependencyClosure(graph *runner.TaskGraph, targets []string) map[string]bool {
	needed := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if needed[name] {
			return
		}
		needed[name] = true
		if task, ok := graph.Task(name); ok {
			for _, dep := range task.Depends {
				visit(dep)
			}
		}
	}
	for _, name := range targets {
		visit(name)
	}
	return needed
}

// Write prints the document. runErr is an error that stopped the run
// before or outside its tasks, such as a configuration that failed to load;
// task failures are reported per task instead.
func (r *JSONReporter) Write(exitCode int, warnings []string, runErr error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	finished := r.opts.Now()
	doc := JSONDocument{
		Version:    JSONVersion,
		Targets:    nonNil(r.opts.Targets),
		Flake:      r.opts.Flake,
		System:     r.opts.System,
		StartedAt:  JSONTime(r.started),
		FinishedAt: JSONTime(finished),
		DurationMs: finished.Sub(r.started).Milliseconds(),
		ExitCode:   exitCode,
		Warnings:   nonNil(warnings),
		Tasks:      nonNil(r.tasks),
	}
	if runErr != nil {
		msg := runErr.Error()
		doc.Error = &msg
	}
	for _, t := range doc.Tasks {
		switch {
		case t.Cached:
			doc.Counts.Cached++
		case t.State == jsonStates[runner.Succeeded]:
			doc.Counts.Passed++
		case t.State == jsonStates[runner.Failed]:
			doc.Counts.Failed++
		case t.State == jsonStates[runner.FailedAllowed]:
			doc.Counts.AllowedFailures++
		case t.State == jsonStates[runner.Skipped]:
			doc.Counts.Skipped++
		case t.State == jsonStates[runner.Interrupted]:
			doc.Counts.Interrupted++
		}
	}
	enc := json.NewEncoder(r.opts.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

var jsonStates = map[runner.TaskState]string{
	runner.Succeeded:     "succeeded",
	runner.Failed:        "failed",
	runner.FailedAllowed: "failed_allowed",
	runner.Skipped:       "skipped",
	runner.Interrupted:   "interrupted",
}

func jsonTask(res runner.TaskResult, label string) JSONTask {
	t := JSONTask{
		Name:   res.Name,
		Label:  label,
		State:  jsonStates[res.State],
		Cached: res.Cached,
	}
	if res.State != runner.Skipped {
		t.StartedAt = ptr(JSONTime(res.Start))
		t.FinishedAt = ptr(JSONTime(res.Start.Add(res.Duration)))
		t.DurationMs = ptr(res.Duration.Milliseconds())
	}
	switch res.State {
	case runner.Succeeded:
		if !res.Cached {
			t.ExitCode = ptr(0)
		}
	case runner.Failed, runner.FailedAllowed:
		if code := res.ExitCode(); code > 0 {
			t.ExitCode = &code
		}
	}
	if res.Error != nil && res.State != runner.Succeeded {
		t.Error = ptr(res.Error.Error())
	}
	if res.SkipReason != "" {
		t.SkipReason = ptr(res.SkipReason)
	}
	if res.Log != "" {
		t.Log = ptr(res.Log)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
