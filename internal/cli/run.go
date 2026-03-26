package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

const failedOutputTailLines = 50

// RunCmd runs a task
type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            int    `short:"j" help:"Number of parallel jobs" default:"4"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
	Stream          bool   `help:"Stream task output in real-time (default in CI)"`
	Force           bool   `help:"Force re-run tasks even if cached"`
	NoCache         bool   `help:"Disable caching (don't read or write cache)"`
	Raw             bool   `help:"Pipe task output directly to stdout/stderr with no decoration"`
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize Nix evaluator
	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	// Load configuration
	slog.Debug("loading config", "flake", globals.Flake)
	configStart := time.Now()
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	slog.Debug("config loaded", "duration", time.Since(configStart), "tasks", len(cfg.Tasks))

	// Validate task exists
	if _, ok := cfg.Tasks[c.Task]; !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Build dependency graph
	slog.Debug("building task graph")
	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}
	slog.Debug("task graph built")

	// Determine failure strategy
	strategy := runner.FailFast
	if c.ContinueOnError {
		strategy = runner.ContinueOnError
	}

	// Determine output mode
	outputMode := runner.DetectOutputMode()
	verbose := !c.Raw && (globals.Verbose || c.Stream || outputMode == runner.Streaming)

	// Create progress display for interactive terminals (not in raw or verbose mode)
	var progress *ui.ProgressDisplay
	useProgress := !c.Raw && !verbose && outputMode == runner.Progress
	if useProgress {
		progress = ui.NewProgressDisplay(ui.ProgressDisplayOptions{
			TailLines: 5,
		})
	}

	// Modified signal handler: clean up progress display before cancelling
	go func() {
		<-sigCh
		if progress != nil {
			progress.Stop()
		}
		cancel()
	}()

	// Compute project key for caching
	projectKey := ""
	if !c.NoCache {
		projectKey = cache.ProjectKey(globals.Flake)
	}

	// In raw mode, only the target task's output pipes directly
	rawTarget := ""
	if c.Raw {
		rawTarget = c.Task
	}

	// Start progress display before execution
	if progress != nil {
		progress.Start()
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
			RawTarget:  rawTarget,
			Progress:   progress,
		},
		runner.ParallelExecutorOptions{
			MaxJobs:  c.Jobs,
			Strategy: strategy,
		},
	)

	slog.Debug("starting execution", "task", c.Task)
	results, err := parallel.ExecuteDAG(ctx, graph, c.Task)

	// Stop progress display (does final render, restores cursor)
	if progress != nil {
		progress.Stop()
	}

	if err != nil {
		return err
	}

	// Print results (skip in raw mode)
	if !c.Raw {
		if useProgress {
			// Progress display already showed per-task status; print summary then failed output
			fmt.Println()
			printSummary(results)
			for _, r := range results {
				if !r.Success && r.Output != "" && !errors.Is(r.Error, context.Canceled) {
					printFailedOutput(r)
				}
			}
		} else {
			fmt.Println()
			printTree(graph, c.Task, results)
		}
	}

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
	printSummary(results)
}

// printSummary prints the "Completed: N tasks (...)" summary line
func printSummary(results []runner.TaskResult) {
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
		tail, tmpFile := tailAndSave(r.Name, r.Output)
		for _, line := range strings.Split(strings.TrimRight(tail, "\n"), "\n") {
			fmt.Printf("%s    %s\n", indent, line)
		}
		if tmpFile != "" {
			fmt.Printf("%s    %s\n", indent, ui.Gray("full output: "+tmpFile))
		}
	}

	// Print children (dependencies)
	deps := graph.Dependencies(name)
	sort.Strings(deps)
	for _, dep := range deps {
		printTreeNode(graph, dep, results, depth+1)
	}
}

// printFailedOutput prints the tail of a failed task's output and its temp log file path
func printFailedOutput(r runner.TaskResult) {
	tail, tmpFile := tailAndSave(r.Name, r.Output)
	fmt.Printf("%s %s\n", ui.Red("✗"), r.Name)
	for _, line := range strings.Split(strings.TrimRight(tail, "\n"), "\n") {
		fmt.Printf("    %s\n", line)
	}
	if tmpFile != "" {
		fmt.Printf("    %s\n", ui.Gray("full output: "+tmpFile))
	}
}

// tailAndSave returns the last failedOutputTailLines lines of output, and writes
// the full output to a temp file. Returns the tail string and the temp file path
// (empty if writing failed or output was short enough to not need truncation).
func tailAndSave(taskName, output string) (tail string, tmpFile string) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Write full output to a temp file
	if f, err := os.CreateTemp("", "nix-tasks-"+taskName+"-*.log"); err == nil {
		f.WriteString(output)
		f.Close()
		tmpFile = f.Name()
	}

	if len(lines) > failedOutputTailLines {
		omitted := len(lines) - failedOutputTailLines
		lines = append([]string{ui.Gray(fmt.Sprintf("... %d lines omitted ...", omitted))}, lines[len(lines)-failedOutputTailLines:]...)
	}
	return strings.Join(lines, "\n"), tmpFile
}
