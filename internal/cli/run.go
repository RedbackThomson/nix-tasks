package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// RunCmd runs a task
type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            int    `short:"j" help:"Number of parallel jobs" default:"4"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
	Stream          bool   `help:"Stream task output in real-time (default in CI)"`
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Initialize Nix evaluator
	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	// Load configuration
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate task exists
	if _, ok := cfg.Tasks[c.Task]; !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Build dependency graph
	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}

	// Determine failure strategy
	strategy := runner.FailFast
	if c.ContinueOnError {
		strategy = runner.ContinueOnError
	}

	// Determine output verbosity
	// Use streaming if explicitly requested, or if in CI, or if verbose
	verbose := globals.Verbose || c.Stream || runner.DetectOutputMode() == runner.Streaming

	// Execute tasks
	parallel := runner.NewParallelExecutor(
		eval,
		cfg,
		runner.ExecutorOptions{
			Verbose: verbose,
			Debug:   globals.Debug,
		},
		runner.ParallelExecutorOptions{
			MaxJobs:  c.Jobs,
			Strategy: strategy,
		},
	)

	results, err := parallel.ExecuteDAG(ctx, graph, c.Task)
	if err != nil {
		return err
	}

	// Print summary
	printSummary(results)

	// Return error if any task failed
	for _, r := range results {
		if !r.Success && !errors.Is(r.Error, context.Canceled) {
			return fmt.Errorf("one or more tasks failed")
		}
	}

	return nil
}

// printSummary prints the execution summary
func printSummary(results []runner.TaskResult) {
	var passed, failed, skipped int
	for _, r := range results {
		if r.Success {
			passed++
		} else if errors.Is(r.Error, context.Canceled) {
			skipped++
		} else {
			failed++
		}
	}

	fmt.Printf("\nCompleted: %d tasks", len(results))
	if passed > 0 {
		fmt.Printf(" (%d passed", passed)
		if failed > 0 {
			fmt.Printf(", %s", ui.Red(fmt.Sprintf("%d failed", failed)))
		}
		if skipped > 0 {
			fmt.Printf(", %d skipped", skipped)
		}
		fmt.Print(")")
	}
	fmt.Println()
}
