package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// RunCmd runs a task
type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            int    `short:"j" help:"Number of tasks to run at once (default: the number of CPUs available)"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
	Force           bool   `help:"Force re-run tasks even if cached"`
	NoCache         bool   `help:"Disable caching (don't read or write cache)"`
	Raw             bool   `help:"Pipe task output directly to stdout/stderr with no decoration"`
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	// Catch signals before loading config so an early Ctrl-C still exits 130.
	// After the first signal the default handlers are restored, so a second
	// Ctrl-C kills nix-tasks without waiting for tasks to shut down.
	reporter := c.newReporter(globals)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			reporter.Interrupt()
			stop()
		case <-finished:
		}
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

	// Compute project key for caching
	projectKey := ""
	if !c.NoCache {
		projectKey = cache.ProjectKey(globals.Flake)
	}

	reporter.Start()
	defer reporter.Stop()
	executor := runner.NewExecutor(eval, cfg, runner.Options{
		Jobs:            c.Jobs,
		ContinueOnError: c.ContinueOnError,
		Force:           c.Force,
		NoCache:         c.NoCache,
		ProjectKey:      projectKey,
		Reporter:        reporter,
		FlakeRoot:       globals.FlakeRoot,
	})

	slog.Debug("starting execution", "task", c.Task)
	results, err := executor.Run(ctx, graph, c.Task)
	if err != nil {
		return err
	}
	reporter.Summary(graph, []string{c.Task}, results)

	code := runExitCode(results)
	switch {
	case code == exitInterrupted || (code == 0 && ctx.Err() != nil):
		return errInterrupted
	case code != 0:
		return &ExitCodeError{Code: code, Err: errors.New("one or more tasks failed")}
	}
	return nil
}

// newReporter picks how the run is shown: raw output for --raw, prefixed
// streamed lines for -v and CI, a live display on a terminal, and start and
// finish lines when piped.
func (c *RunCmd) newReporter(globals *Globals) ui.Reporter {
	switch {
	case c.Raw:
		return ui.NewRawReporter(c.Task, os.Stdout, os.Stderr)
	case globals.Verbose || os.Getenv("CI") != "":
		return ui.NewLineReporter(os.Stdout, true)
	case ui.IsTerminal(os.Stdout):
		return ui.NewProgressReporter(ui.ProgressOptions{Out: os.Stdout})
	}
	return ui.NewLineReporter(os.Stdout, false)
}
