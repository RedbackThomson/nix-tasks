package runner

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// TaskState is the outcome of a task in a run
type TaskState int

const (
	Succeeded TaskState = iota
	Failed
	// FailedAllowed is a failure of a task with continueOnError; its
	// dependents still run and it does not fail the run.
	FailedAllowed
	// Skipped tasks never started, because a dependency did not succeed or
	// the run was stopped first.
	Skipped
	// Interrupted tasks were killed because the run was cancelled.
	Interrupted
)

func (s TaskState) String() string {
	switch s {
	case Succeeded:
		return "succeeded"
	case Failed:
		return "failed"
	case FailedAllowed:
		return "failed (allowed)"
	case Skipped:
		return "skipped"
	case Interrupted:
		return "interrupted"
	}
	return "unknown"
}

// TaskResult holds the result of a task execution
type TaskResult struct {
	Name     string
	State    TaskState
	Cached   bool
	Error    error
	Duration time.Duration
	Output   string // Buffered stdout/stderr (non-verbose mode only)
}

// Run executes target and everything it needs, starting each task as soon as
// all of its dependencies have succeeded. Every task in the execution set gets
// a result, returned in execution order.
func (e *Executor) Run(ctx context.Context, graph *TaskGraph, target string) ([]TaskResult, error) {
	order, err := graph.ExecutionOrder(target)
	if err != nil {
		return nil, err
	}

	remaining := make(map[string]int, len(order))
	dependents := make(map[string][]string, len(order))
	for _, name := range order {
		deps := slices.Clone(graph.Dependencies(name))
		slices.Sort(deps)
		deps = slices.Compact(deps)
		remaining[name] = len(deps)
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var ready []string
	for _, name := range order {
		if remaining[name] == 0 {
			ready = append(ready, name)
		}
	}

	results := make(map[string]TaskResult, len(order))
	var resolve func(r TaskResult)
	resolve = func(r TaskResult) {
		results[r.Name] = r
		next := slices.Clone(dependents[r.Name])
		slices.Sort(next)
		for _, d := range next {
			if _, done := results[d]; done {
				continue
			}
			if r.State == Succeeded || r.State == FailedAllowed {
				remaining[d]--
				if remaining[d] == 0 {
					ready = append(ready, d)
				}
				continue
			}
			resolve(e.skip(d))
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan TaskResult)
	running := 0
	for {
		slices.Sort(ready)
		for runCtx.Err() == nil && running < e.options.Jobs && len(ready) > 0 {
			name := ready[0]
			ready = ready[1:]
			running++
			if e.options.Progress != nil {
				e.options.Progress.RegisterTask(name)
			}
			task := graph.tasks[name]
			go func() {
				r := e.runOne(runCtx, name, task)
				if r.State == Failed && !e.options.ContinueOnError {
					cancel()
				}
				done <- r
			}()
		}
		if running == 0 {
			break
		}
		r := <-done
		running--
		resolve(r)
	}

	out := make([]TaskResult, 0, len(order))
	for _, name := range order {
		r, ok := results[name]
		if !ok {
			r = e.skip(name)
		}
		out = append(out, r)
	}
	return out, nil
}

// skip records a task that will not run
func (e *Executor) skip(name string) TaskResult {
	if e.options.Progress != nil {
		e.options.Progress.RegisterTask(name)
		e.options.Progress.TaskSkipped(name)
	}
	return TaskResult{Name: name, State: Skipped}
}

// runOne runs a single task and classifies its outcome
func (e *Executor) runOne(ctx context.Context, name string, task config.Task) TaskResult {
	start := time.Now()
	cached, err := e.runTask(ctx, name, task)
	duration := time.Since(start)

	state := Succeeded
	switch {
	case err == nil:
	case ctx.Err() != nil:
		state = Interrupted
	case task.ContinueOnError:
		state = FailedAllowed
	default:
		state = Failed
	}

	var output string
	if e.options.Progress != nil {
		e.options.Progress.TaskFinished(name, progressStatus(state, cached), duration)
		output = e.options.Progress.GetBuffer(name)
	} else {
		output = e.printer.GetBuffer(name)
	}

	return TaskResult{
		Name:     name,
		State:    state,
		Cached:   cached,
		Error:    err,
		Duration: duration,
		Output:   output,
	}
}

func progressStatus(state TaskState, cached bool) ui.TaskStatus {
	switch state {
	case Failed:
		return ui.TaskFailed
	case FailedAllowed:
		return ui.TaskFailedAllowed
	case Interrupted:
		return ui.TaskInterrupted
	case Skipped:
		return ui.TaskSkipped
	}
	if cached {
		return ui.TaskCached
	}
	return ui.TaskSuccess
}

// runTask executes a single task, answering from the cache when it can. Only
// successful runs are cached, so a failure is always retried.
func (e *Executor) runTask(ctx context.Context, name string, task config.Task) (cached bool, err error) {
	var fp *cache.Fingerprint
	if e.cache != nil {
		var fpErr error
		fp, fpErr = cache.ComputeFingerprint(task, e.config.Packages, e.taskDir(task))
		if fpErr != nil {
			slog.Debug("failed to compute fingerprint", "task", name, "error", fpErr)
		}
		if fp == nil && !task.NoCache && task.Type != config.TaskTypeBuild && len(task.Inputs) == 0 {
			slog.Debug("skipping cache: shell task has no declared inputs", "task", name)
		}
	}

	if fp != nil && !e.options.Force {
		if entry, ok := e.cache.Lookup(name, fp); ok && entry.Success {
			return true, nil
		}
	}

	if err := e.execTask(ctx, name, task); err != nil {
		return false, &TaskExecutionError{Name: name, Err: err}
	}

	if fp != nil {
		if storeErr := e.cache.Store(name, fp, true); storeErr != nil {
			slog.Debug("failed to store cache entry", "task", name, "error", storeErr)
		}
	}
	return false, nil
}

// TaskExecutionError indicates a task failed during execution
type TaskExecutionError struct {
	Name string
	Err  error
}

func (e *TaskExecutionError) Error() string {
	return "task '" + e.Name + "' failed: " + e.Err.Error()
}

func (e *TaskExecutionError) Unwrap() error {
	return e.Err
}
