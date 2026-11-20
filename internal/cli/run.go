package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	Jobs            int    `short:"j" help:"Number of tasks to run at once (default: the number of CPUs available)"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
	Stream          bool   `help:"Stream task output in real-time (default in CI)"`
	Force           bool   `help:"Force re-run tasks even if cached"`
	NoCache         bool   `help:"Disable caching (don't read or write cache)"`
	Raw             bool   `help:"Pipe task output directly to stdout/stderr with no decoration"`
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	// Catch signals before loading config so an early Ctrl-C still exits 130.
	// After the first signal the default handlers are restored, so a second
	// Ctrl-C kills nix-tasks without waiting for tasks to shut down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()

	// Initialize Nix evaluator
	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	// Load configuration
	slog.Debug("loading config", "flake", globals.Flake)
	configStart := time.Now()
	cfg, err := config.Load(ctx, eval)
	if ctx.Err() != nil {
		return errInterrupted
	}
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

	executor := runner.NewExecutor(eval, cfg, runner.Options{
		Jobs:            c.Jobs,
		ContinueOnError: c.ContinueOnError,
		Verbose:         verbose,
		Force:           c.Force,
		NoCache:         c.NoCache,
		ProjectKey:      projectKey,
		RawTarget:       rawTarget,
		Progress:        progress,
		FlakeRoot:       globals.FlakeRoot,
	})

	slog.Debug("starting execution", "task", c.Task)
	results, err := executor.Run(ctx, graph, c.Task)

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
				if showsOutput(r) {
					printFailedOutput(r)
				}
			}
		} else {
			fmt.Println()
			resultMap := make(map[string]runner.TaskResult, len(results))
			execSet := make(map[string]bool, len(results))
			for _, r := range results {
				resultMap[r.Name] = r
				execSet[r.Name] = true
			}
			printTree(graph, c.Task, execSet, resultMap)
			printSummary(results)
		}
	}

	code := runExitCode(results)
	switch {
	case code == exitInterrupted || (code == 0 && ctx.Err() != nil):
		return errInterrupted
	case code != 0:
		return &ExitCodeError{Code: code, Err: errors.New("one or more tasks failed")}
	}
	return nil
}

// printSummary prints the "Completed: N tasks (...)" summary line
func printSummary(results []runner.TaskResult) {
	counts := map[runner.TaskState]int{}
	for _, r := range results {
		counts[r.State]++
	}

	fmt.Printf("Completed: %d tasks", len(results))
	parts := []string{}
	if n := counts[runner.Succeeded]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", n))
	}
	if n := counts[runner.Failed]; n > 0 {
		parts = append(parts, ui.Failure(fmt.Sprintf("%d failed", n)))
	}
	if n := counts[runner.FailedAllowed]; n > 0 {
		parts = append(parts, ui.Warning(fmt.Sprintf("%d failed (allowed)", n)))
	}
	if n := counts[runner.Skipped]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", n))
	}
	if n := counts[runner.Interrupted]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d interrupted", n))
	}
	if len(parts) > 0 {
		fmt.Printf(" (%s)", strings.Join(parts, ", "))
	}
	fmt.Println()
}

func stateIcon(state runner.TaskState) string {
	switch state {
	case runner.Failed:
		return ui.Failure("✗")
	case runner.FailedAllowed:
		return ui.Warning("!")
	case runner.Skipped:
		return ui.Muted("-")
	case runner.Interrupted:
		return ui.Warning("⏹")
	}
	return ui.Success("✓")
}

// showsOutput reports whether a task's captured output is printed after the
// run. Interrupted output is noise from the kill, not the cause.
func showsOutput(r runner.TaskResult) bool {
	return (r.State == runner.Failed || r.State == runner.FailedAllowed) && r.Output != ""
}

// printFailedOutput prints the tail of a failed task's output and its temp log file path
func printFailedOutput(r runner.TaskResult) {
	tail, tmpFile := tailAndSave(r.Name, r.Output)
	fmt.Printf("%s %s\n", stateIcon(r.State), r.Name)
	for _, line := range strings.Split(strings.TrimRight(tail, "\n"), "\n") {
		fmt.Printf("    %s\n", line)
	}
	if tmpFile != "" {
		fmt.Printf("    %s\n", ui.Muted("full output: "+tmpFile))
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
		lines = append([]string{ui.Muted(fmt.Sprintf("... %d lines omitted ...", omitted))}, lines[len(lines)-failedOutputTailLines:]...)
	}
	return strings.Join(lines, "\n"), tmpFile
}
