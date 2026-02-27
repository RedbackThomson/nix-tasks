package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/redbackthomson/nix-tasks/internal/cache"
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
	Force           bool   `help:"Force re-run tasks even if cached"`
	NoCache         bool   `help:"Disable caching (don't read or write cache)"`
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

	// Compute project key for caching
	projectKey := ""
	if !c.NoCache {
		projectKey = cache.ProjectKey(globals.Flake)
	}

	// Execute tasks
	parallel := runner.NewParallelExecutor(
		eval,
		cfg,
		runner.ExecutorOptions{
			Verbose:    verbose,
			Debug:      globals.Debug,
			Force:      c.Force,
			NoCache:    c.NoCache,
			ProjectKey: projectKey,
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

	// Print dependency tree with results
	fmt.Println()
	printTree(graph, c.Task, results)

	// Return error if any task failed
	for _, r := range results {
		if !r.Success && !errors.Is(r.Error, context.Canceled) {
			return fmt.Errorf("one or more tasks failed")
		}
	}

	return nil
}

// printTree prints an ASCII dependency tree with results and a summary line
func printTree(graph *runner.TaskGraph, target string, results []runner.TaskResult) {
	resultMap := make(map[string]runner.TaskResult)
	for _, r := range results {
		resultMap[r.Name] = r
	}

	printTreeNode(graph, target, resultMap, 0)

	// Print summary line
	var passed, failed, skipped int
	for _, r := range results {
		switch {
		case r.Success:
			passed++
		case errors.Is(r.Error, context.Canceled):
			skipped++
		default:
			failed++
		}
	}

	fmt.Printf("Completed: %d tasks", len(results))
	parts := []string{}
	if passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", passed))
	}
	if failed > 0 {
		parts = append(parts, ui.Red(fmt.Sprintf("%d failed", failed)))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) > 0 {
		fmt.Printf(" (%s)", strings.Join(parts, ", "))
	}
	fmt.Println()
}

// printTreeNode recursively prints a task and its dependencies as an indented tree
func printTreeNode(graph *runner.TaskGraph, name string, results map[string]runner.TaskResult, depth int) {
	r, ok := results[name]
	indent := strings.Repeat("  ", depth)

	// Determine status symbol
	status := ui.Green("✓")
	if ok && !r.Success {
		if errors.Is(r.Error, context.Canceled) {
			status = ui.Gray("-")
		} else {
			status = ui.Red("✗")
		}
	}

	// Determine suffix (duration or cached)
	suffix := ""
	if ok {
		if r.Cached {
			suffix = " " + ui.Gray("(cached)")
		} else if r.Duration > 0 {
			suffix = " " + ui.Gray(ui.FormatDuration(r.Duration))
		}
	}

	fmt.Printf("%s%s %s%s\n", indent, status, name, suffix)

	// Print buffered output for failed tasks
	if ok && !r.Success && r.Output != "" && !errors.Is(r.Error, context.Canceled) {
		for _, line := range strings.Split(strings.TrimRight(r.Output, "\n"), "\n") {
			fmt.Printf("%s    %s\n", indent, line)
		}
	}

	// Print children (dependencies)
	deps := graph.Dependencies(name)
	sort.Strings(deps)
	for _, dep := range deps {
		printTreeNode(graph, dep, results, depth+1)
	}
}
