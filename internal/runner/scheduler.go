package runner

import (
	"context"
	"errors"
	"maps"
	"os/exec"
	"slices"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
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
	Name  string
	State TaskState
	// Cached is set when a cache entry answered the task instead of a run.
	Cached bool
	// Saved is how long the run that stored the answering entry took; zero
	// unless Cached.
	Saved time.Duration
	// Cache describes the cache decision for a task that declares a cache;
	// nil for other tasks and when the cache is disabled.
	Cache *CacheOutcome
	// Warnings are problems that did not fail the task.
	Warnings []string
	Error    error
	// SkipReason says why a skipped task never started, e.g. "lint failed".
	SkipReason string
	// Start is when the task began; zero for skipped tasks.
	Start    time.Time
	Duration time.Duration
	// Log is the file holding the task's combined output, or "" when the
	// task did not run (cached or skipped) or the run keeps no logs.
	Log string

	// key is the task's cache key, published to its dependents once the
	// task has hit or run to completion with stable inputs.
	key string
}

// ExitCode returns the exit status of the task's process, or -1 when the
// task did not fail because a process exited non-zero.
func (r TaskResult) ExitCode() int {
	var exitErr *exec.ExitError
	if errors.As(r.Error, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return -1
}

// Run executes the targets and everything they need, starting each task as
// soon as all of its dependencies have succeeded. Every task in the execution
// set gets a result, returned in execution order.
func (e *Executor) Run(ctx context.Context, graph *TaskGraph, targets ...string) ([]TaskResult, error) {
	order, err := graph.ExecutionOrder(targets...)
	if err != nil {
		return nil, err
	}

	var needShell []string
	for _, name := range order {
		task := graph.tasks[name]
		if task.Type != config.TaskTypeBuild && task.ShellDrvPath == "" && taskScript(task) != "" {
			needShell = append(needShell, name)
		}
	}
	e.devEnvs = newDevEnvs(e.nix, e.options.DevEnvDir, needShell)
	e.evalBuildDrvs(ctx, graph, order)

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

	if p, ok := e.reporter.(PlanReporter); ok {
		p.RunPlanned(RunPlan{
			Targets: targets,
			Tasks:   planOrder(ready, remaining, dependents),
			Labels:  config.Labels(e.config.Tasks),
			Jobs:    e.options.Jobs,
		})
	}

	if s, ok := e.reporter.(SetupReporter); ok {
		start := time.Now()
		envs := e.prepareEnvs(ctx, graph, order)
		s.SetupFinished(envs, time.Since(start))
	}

	results := make(map[string]TaskResult, len(order))
	keys := make(map[string]string, len(order))
	finish := func(r TaskResult) {
		results[r.Name] = r
		if r.key != "" {
			keys[r.Name] = r.key
		}
		e.reporter.TaskFinished(r)
	}
	var resolve func(r TaskResult)
	resolve = func(r TaskResult) {
		finish(r)
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
			resolve(skip(d, r.Name+" "+r.State.String()))
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
			task := graph.tasks[name]
			plan := e.planCache(graph, name, results, keys)
			go func() {
				r := e.runOne(runCtx, name, task, plan)
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

	for _, name := range order {
		if _, ok := results[name]; !ok {
			finish(skip(name, "run stopped"))
		}
	}
	out := make([]TaskResult, 0, len(order))
	for _, name := range order {
		out = append(out, results[name])
	}
	return out, nil
}

// planOrder sorts the execution set topologically, taking the
// alphabetically first ready task at each step.
func planOrder(ready []string, remaining map[string]int, dependents map[string][]string) []string {
	left := maps.Clone(remaining)
	queue := slices.Clone(ready)
	var order []string
	for len(queue) > 0 {
		slices.Sort(queue)
		name := queue[0]
		queue = queue[1:]
		order = append(order, name)
		for _, d := range dependents[name] {
			left[d]--
			if left[d] == 0 {
				queue = append(queue, d)
			}
		}
	}
	return order
}

func skip(name, reason string) TaskResult {
	return TaskResult{Name: name, State: Skipped, SkipReason: reason}
}

// runOne runs a single task and classifies its outcome
func (e *Executor) runOne(ctx context.Context, name string, task config.Task, plan cachePlan) TaskResult {
	r := TaskResult{Name: name, Start: time.Now()}
	err := e.runTask(ctx, name, task, plan, &r)
	r.Duration = time.Since(r.Start)

	switch {
	case err == nil:
		r.State = Succeeded
	case ctx.Err() != nil:
		r.State = Interrupted
	case task.ContinueOnError:
		r.State = FailedAllowed
	default:
		r.State = Failed
	}
	if err != nil {
		r.Error = &TaskExecutionError{Name: name, Err: err}
		r.key = ""
	}
	return r
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
