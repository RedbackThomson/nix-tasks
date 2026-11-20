package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
)

// TaskResult holds the result of a task execution
type TaskResult struct {
	Name     string
	Success  bool
	Cached   bool
	Error    error
	Duration time.Duration
	Output   string // Buffered stdout/stderr (non-verbose mode only)
}

// Run executes all tasks required for target, respecting dependencies
func (p *Executor) Run(ctx context.Context, graph *TaskGraph, target string) ([]TaskResult, error) {
	groups, err := graph.ParallelGroups(target)
	if err != nil {
		return nil, err
	}

	var results []TaskResult
	var failed bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, group := range groups {
		// Check if we should stop due to prior failure
		if failed && !p.options.ContinueOnError {
			results = append(results, p.skipTasks(group, context.Canceled)...)
			continue
		}

		// Check for context cancellation before starting group
		if ctx.Err() != nil {
			results = append(results, p.skipTasks(group, ctx.Err())...)
			break
		}

		groupResults := p.executeGroup(ctx, graph, group)
		results = append(results, groupResults...)

		// Check for failures
		for _, r := range groupResults {
			if !r.Success {
				failed = true
				if !p.options.ContinueOnError {
					cancel()
					break
				}
			}
		}
	}

	return results, nil
}

// executeGroup runs a group of independent tasks in parallel
func (p *Executor) executeGroup(ctx context.Context, graph *TaskGraph, tasks []string) []TaskResult {
	results := make([]TaskResult, len(tasks))
	sem := make(chan struct{}, p.options.Jobs)
	var wg sync.WaitGroup

	for i, name := range tasks {
		// Register task with progress display before launching goroutine
		if p.options.Progress != nil {
			p.options.Progress.RegisterTask(name)
		}

		wg.Add(1)
		go func(idx int, taskName string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = TaskResult{
					Name:    taskName,
					Success: false,
					Error:   ctx.Err(),
				}
				if p.options.Progress != nil {
					p.options.Progress.TaskSkipped(taskName)
				}
				return
			}

			// Check if context cancelled while waiting for semaphore
			if ctx.Err() != nil {
				results[idx] = TaskResult{
					Name:    taskName,
					Success: false,
					Error:   ctx.Err(),
				}
				if p.options.Progress != nil {
					p.options.Progress.TaskSkipped(taskName)
				}
				return
			}

			// ParallelGroups only yields names from the graph, so the lookup cannot miss.
			task := graph.tasks[taskName]

			// Execute task with timing
			start := time.Now()
			cached, err := p.runTask(ctx, taskName, task)
			duration := time.Since(start)

			// Notify progress display of completion
			if p.options.Progress != nil {
				p.options.Progress.TaskFinished(taskName, err, duration, cached)
			}

			// Get buffered output from the appropriate source
			output := ""
			if p.options.Progress != nil {
				output = p.options.Progress.GetBuffer(taskName)
			} else {
				output = p.printer.GetBuffer(taskName)
			}

			results[idx] = TaskResult{
				Name:     taskName,
				Success:  err == nil,
				Cached:   cached,
				Error:    err,
				Duration: duration,
				Output:   output,
			}
		}(i, name)
	}

	wg.Wait()
	return results
}

// runTask executes a single task with caching support
func (p *Executor) runTask(ctx context.Context, name string, task config.Task) (cached bool, err error) {
	// Compute fingerprint for caching
	var fp *cache.Fingerprint
	if p.cache != nil {
		var fpErr error
		fp, fpErr = cache.ComputeFingerprint(task, p.config.Packages, p.taskDir(task))
		if fpErr != nil {
			slog.Debug("failed to compute fingerprint", "task", name, "error", fpErr)
			// Continue without caching
		}
		if fp == nil && !task.NoCache && task.Type != config.TaskTypeBuild && len(task.Inputs) == 0 {
			slog.Debug("skipping cache: shell task has no declared inputs", "task", name)
		}
	}

	// Check cache if not forcing rebuild
	if fp != nil && !p.options.Force {
		if entry, ok := p.cache.Lookup(name, fp); ok {
			if !entry.Success {
				return true, &TaskExecutionError{Name: name, Err: fmt.Errorf("failed (cached)")}
			}
			return true, nil
		}
	}

	// Execute the task using the executor (handles both shell and build tasks)
	execErr := p.execTask(ctx, name, task)

	// Store result in cache (even failures, so we can skip them next time)
	if fp != nil {
		if storeErr := p.cache.Store(name, fp, execErr == nil); storeErr != nil {
			slog.Debug("failed to store cache entry", "task", name, "error", storeErr)
		}
	}

	if execErr != nil {
		// Check if task allows continuing on error
		if task.ContinueOnError {
			return false, nil // Don't propagate error
		}
		return false, &TaskExecutionError{Name: name, Err: execErr}
	}

	return false, nil
}

// skipTasks marks a group of tasks as skipped and returns their results
func (p *Executor) skipTasks(tasks []string, err error) []TaskResult {
	results := make([]TaskResult, len(tasks))
	for i, name := range tasks {
		if p.options.Progress != nil {
			p.options.Progress.RegisterTask(name)
			p.options.Progress.TaskSkipped(name)
		}
		results[i] = TaskResult{
			Name:    name,
			Success: false,
			Error:   err,
		}
	}
	return results
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
