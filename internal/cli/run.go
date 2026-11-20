package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// RunCmd runs a task
type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            *int   `short:"j" help:"Number of tasks to run at once (default: $NIX_TASKS_JOBS, or else the number of CPUs available)"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
	Force           bool   `help:"Force re-run tasks even if cached"`
	NoCache         bool   `help:"Disable caching (don't read or write cache)"`
	Raw             bool   `help:"Pipe task output directly to stdout/stderr with no decoration"`
}

// Validate is called by kong after parsing.
func (c *RunCmd) Validate() error {
	jobs, err := resolveJobs(c.Jobs, os.Getenv("NIX_TASKS_JOBS"))
	if err != nil {
		return err
	}
	c.Jobs = &jobs
	return nil
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

	slog.Debug("loading config", "flake", globals.Flake)
	configStart := time.Now()
	cfg, eval, err := loadConfig(ctx, globals)
	if ctx.Err() != nil {
		return errInterrupted
	}
	if err != nil {
		return err
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
		Jobs:            *c.Jobs,
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

// resolveJobs returns how many tasks run at once: -j if given, else
// NIX_TASKS_JOBS if set, else one per CPU nix-tasks may use.
func resolveJobs(flag *int, env string) (int, error) {
	if flag != nil {
		if *flag < 1 {
			return 0, fmt.Errorf("--jobs must be at least 1, got %d", *flag)
		}
		return *flag, nil
	}
	if env == "" {
		return runtime.GOMAXPROCS(0), nil
	}
	n, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("NIX_TASKS_JOBS must be a whole number, got %q", env)
	}
	if n < 1 {
		return 0, fmt.Errorf("NIX_TASKS_JOBS must be at least 1, got %d", n)
	}
	return n, nil
}
