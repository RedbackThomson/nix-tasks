package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// TaskResult holds the result of a task execution
type TaskResult struct {
	Name     string
	Success  bool
	Cached   bool
	Error    error
	Duration time.Duration
}

// ParallelExecutor runs tasks with parallelism
type ParallelExecutor struct {
	executor *Executor // Delegate to executor for task execution
	nix      *nix.Evaluator
	config   *config.Config
	options  ExecutorOptions
	printer  *ui.Printer
	cache    *cache.Store
	maxJobs  int
	strategy FailureStrategy
}

// ParallelExecutorOptions configures the parallel executor
type ParallelExecutorOptions struct {
	MaxJobs  int
	Strategy FailureStrategy
}

// NewParallelExecutor creates a parallel executor
func NewParallelExecutor(
	eval *nix.Evaluator,
	cfg *config.Config,
	execOpts ExecutorOptions,
	parallelOpts ParallelExecutorOptions,
) *ParallelExecutor {
	maxJobs := parallelOpts.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4 // Default parallelism
	}
	eval.SetDebug(execOpts.Debug)

	// Initialize cache store if caching is enabled
	var cacheStore *cache.Store
	if !execOpts.NoCache && execOpts.ProjectKey != "" {
		cacheStore = cache.NewStore(execOpts.ProjectKey)
	}

	// Create an executor for task execution
	executor := NewExecutor(eval, cfg, execOpts)

	return &ParallelExecutor{
		executor: executor,
		nix:      eval,
		config:   cfg,
		options:  execOpts,
		printer:  ui.NewPrinter(),
		cache:    cacheStore,
		maxJobs:  maxJobs,
		strategy: parallelOpts.Strategy,
	}
}

// ExecuteDAG executes all tasks required for target, respecting dependencies
func (p *ParallelExecutor) ExecuteDAG(ctx context.Context, graph *TaskGraph, target string) ([]TaskResult, error) {
	groups, err := graph.ParallelGroups(target)
	if err != nil {
		return nil, err
	}

	var results []TaskResult
	var failed bool
	var cancelOnce sync.Once
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, group := range groups {
		// Check if we should stop due to prior failure
		if failed && p.strategy == FailFast {
			// Mark remaining tasks as skipped
			for _, name := range group {
				results = append(results, TaskResult{
					Name:    name,
					Success: false,
					Error:   context.Canceled,
				})
			}
			continue
		}

		// Check for context cancellation before starting group
		if ctx.Err() != nil {
			for _, name := range group {
				results = append(results, TaskResult{
					Name:    name,
					Success: false,
					Error:   ctx.Err(),
				})
			}
			break
		}

		groupResults := p.executeGroup(ctx, graph, group)
		results = append(results, groupResults...)

		// Check for failures
		for _, r := range groupResults {
			if !r.Success {
				failed = true
				if p.strategy == FailFast {
					// Cancel context for any in-flight tasks
					cancelOnce.Do(cancel)
					break
				}
			}
		}
	}

	return results, nil
}

// executeGroup runs a group of independent tasks in parallel
func (p *ParallelExecutor) executeGroup(ctx context.Context, graph *TaskGraph, tasks []string) []TaskResult {
	results := make([]TaskResult, len(tasks))
	sem := make(chan struct{}, p.maxJobs)
	var wg sync.WaitGroup

	for i, name := range tasks {
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
				return
			}

			// Check if context cancelled while waiting for semaphore
			if ctx.Err() != nil {
				results[idx] = TaskResult{
					Name:    taskName,
					Success: false,
					Error:   ctx.Err(),
				}
				return
			}

			// Get task definition
			task, ok := graph.Task(taskName)
			if !ok {
				results[idx] = TaskResult{
					Name:    taskName,
					Success: false,
					Error:   &TaskNotFoundError{Name: taskName},
				}
				return
			}

			// Execute task with timing
			start := time.Now()
			cached, err := p.runTask(ctx, taskName, task)
			duration := time.Since(start)

			results[idx] = TaskResult{
				Name:     taskName,
				Success:  err == nil,
				Cached:   cached,
				Error:    err,
				Duration: duration,
			}
		}(i, name)
	}

	wg.Wait()
	return results
}

// runTask executes a single task (similar to Executor.RunTask but with duration tracking)
func (p *ParallelExecutor) runTask(ctx context.Context, name string, task config.Task) (cached bool, err error) {
	// Compute fingerprint for caching
	var fp *cache.Fingerprint
	if p.cache != nil {
		var fpErr error
		fp, fpErr = cache.ComputeFingerprint(task, p.config.Packages, task.WorkingDir)
		if fpErr != nil {
			slog.Debug("failed to compute fingerprint", "task", name, "error", fpErr)
			// Continue without caching
		}
	}

	// Check cache if not forcing rebuild
	if fp != nil && !p.options.Force {
		if entry, ok := p.cache.Lookup(name, fp); ok {
			p.printer.TaskCached(name)
			if !entry.Success {
				return true, &TaskExecutionError{Name: name, Err: fmt.Errorf("failed (cached)")}
			}
			return true, nil
		}
	}

	// Execute the task using the executor (handles both shell and build tasks)
	start := time.Now()
	execErr := p.executor.RunTask(ctx, name, task)
	duration := time.Since(start)

	// Store result in cache (even failures, so we can skip them next time)
	if fp != nil {
		if storeErr := p.cache.Store(name, fp, execErr == nil); storeErr != nil {
			slog.Debug("failed to store cache entry", "task", name, "error", storeErr)
		}
	}

	if execErr != nil {
		// Check if task allows continuing on error
		if task.ContinueOnError {
			p.printer.TaskFailedWithDuration(name, execErr, duration)
			return false, nil // Don't propagate error
		}
		p.printer.TaskFailedWithDuration(name, execErr, duration)
		return false, &TaskExecutionError{Name: name, Err: execErr}
	}

	p.printer.TaskSucceededWithDuration(name, duration)
	return false, nil
}

// TaskNotFoundError indicates a task was not found
type TaskNotFoundError struct {
	Name string
}

func (e *TaskNotFoundError) Error() string {
	return "task not found: " + e.Name
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
