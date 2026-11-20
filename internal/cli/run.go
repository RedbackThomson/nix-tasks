package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runlog"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// RunCmd runs one or more tasks
type RunCmd struct {
	Tasks           []string `arg:"" name:"task" completion-predictor:"task" help:"Tasks to run; several run together with one summary"`
	Jobs            *int     `short:"j" help:"Number of tasks to run at once (default: $NIX_TASKS_JOBS, or else the number of CPUs available)"`
	ContinueOnError bool     `help:"Continue running independent tasks on failure"`
	Force           bool     `help:"Run cached tasks without looking them up, refreshing their cache entries"`
	NoCache         bool     `help:"Disable the task cache: nothing is hashed, looked up or stored"`
	Raw             bool     `help:"Pipe task output directly to stdout/stderr with no decoration"`
}

// Validate is called by kong after parsing.
func (c *RunCmd) Validate() error {
	jobs, err := resolveJobs(c.Jobs, os.Getenv("NIX_TASKS_JOBS"))
	if err != nil {
		return err
	}
	c.Jobs = &jobs
	if c.Raw && len(c.Tasks) > 1 {
		return errors.New("--raw runs a single task: its output would be interleaved with the others'")
	}
	return nil
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	if c.Raw && globals.Output == OutputJSON {
		return errors.New("--output=json cannot be combined with --raw: the JSON document is the only thing on stdout")
	}
	warnings := &warnings{out: os.Stderr}
	reporter := c.newReporter(globals)
	err := c.run(globals, reporter, warnings)
	if j, ok := reporter.(*ui.JSONReporter); ok {
		code, runErr := exitStatus(err)
		if werr := j.Write(code, warnings.list(), runErr); werr != nil {
			return errors.Join(err, fmt.Errorf("failed to write the JSON document: %w", werr))
		}
	}
	return err
}

// exitStatus is the exit code nix-tasks ends with for err, and err itself
// when it is not just the outcome of the run's tasks.
func exitStatus(err error) (int, error) {
	var exitErr *ExitCodeError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		return exitErr.Code, nil
	}
	return 1, err
}

func (c *RunCmd) run(globals *Globals, reporter ui.Reporter, warnings *warnings) error {
	// Catch signals before loading config so an early Ctrl-C still exits 130.
	// After the first signal the default handlers are restored, so a second
	// Ctrl-C kills nix-tasks without waiting for tasks to shut down.
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
	if g, ok := reporter.(*ui.GitHubReporter); ok {
		g.ConfigLoaded(time.Since(configStart))
	}

	if err := checkTasksExist(cfg, c.Tasks); err != nil {
		return err
	}

	// Build dependency graph
	slog.Debug("building task graph")
	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}
	slog.Debug("task graph built")

	store, err := c.openCache(ctx, globals, cfg, graph, warnings.add)
	if err != nil {
		return err
	}

	// The JSON document holds no task output, so the logs are where it is.
	logDir := ""
	if _, ok := reporter.(*ui.JSONReporter); ok {
		logDir = startRunLogs(time.Now(), warnings)
	}

	reporter.Start()
	defer reporter.Stop()
	executor := runner.NewExecutor(eval, cfg, runner.Options{
		Jobs:            *c.Jobs,
		ContinueOnError: c.ContinueOnError,
		Force:           c.Force,
		Cache:           store,
		Reporter:        reporter,
		FlakeRoot:       globals.FlakeRoot,
		LogDir:          logDir,
		Warn:            warnings.add,
	})

	slog.Debug("starting execution", "tasks", c.Tasks)
	results, err := executor.Run(ctx, graph, c.Tasks...)
	if err != nil {
		return err
	}
	reporter.Summary(graph, c.Tasks, results)

	code := runExitCode(results)
	switch {
	case code == exitInterrupted || (code == 0 && ctx.Err() != nil):
		return errInterrupted
	case code != 0:
		return &ExitCodeError{Code: code, Err: errors.New("one or more tasks failed")}
	}
	return nil
}

// openCache returns the store for the run, or nil when no task in it
// declares a cache or caching is off. Warnings about cached tasks that can
// never hit go to warnf here, before any task starts.
func (c *RunCmd) openCache(ctx context.Context, globals *Globals, cfg *config.Config, graph *runner.TaskGraph, warnf func(string)) (*cache.Store, error) {
	if c.NoCache {
		return nil, nil
	}
	order, err := graph.ExecutionOrder(c.Tasks...)
	if err != nil {
		return nil, err
	}
	cached := slices.ContainsFunc(order, func(name string) bool { return cfg.Tasks[name].Cache != nil })
	if !cached {
		return nil, nil
	}
	if globals.FlakeRoot == "" {
		warnf(fmt.Sprintf("the task cache is disabled: %s is not a local path", globals.Flake))
		return nil, nil
	}
	for _, name := range order {
		if w := config.AlwaysRunsWarning(cfg, name); w != "" {
			warnf(w)
		}
	}
	store := cache.Open(cache.Dir(), cache.ProjectKey(ctx, globals.FlakeRoot))
	store.RemoveOrphans(func(task string) bool {
		_, ok := cfg.Tasks[task]
		return ok
	})
	return store, nil
}

func warn(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", ui.Warning("warning:"), fmt.Sprintf(format, args...))
}

// newReporter picks how the run is shown: --raw wins over --output.
func (c *RunCmd) newReporter(globals *Globals) ui.Reporter {
	if c.Raw {
		return ui.NewRawReporter(c.Tasks[0], os.Stdout, os.Stderr)
	}
	switch globals.OutputMode() {
	case OutputJSON:
		flake := globals.FlakeRoot
		if flake == "" {
			flake = globals.Flake
		}
		return ui.NewJSONReporter(ui.JSONOptions{Out: os.Stdout, Targets: c.Tasks, Flake: flake, System: nix.CurrentSystem()})
	case OutputGitHub:
		return ui.NewGitHubReporter(ui.GitHubOptions{Out: os.Stdout, SummaryPath: os.Getenv("GITHUB_STEP_SUMMARY")})
	case OutputStream:
		return ui.NewLineReporter(os.Stdout, true)
	case OutputTTY:
		return ui.NewProgressReporter(ui.ProgressOptions{Out: os.Stdout})
	}
	return ui.NewLineReporter(os.Stdout, false)
}

// checkTasksExist names every requested task the config does not define.
func checkTasksExist(cfg *config.Config, names []string) error {
	var missing []string
	for _, name := range names {
		if _, ok := cfg.Tasks[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("task not found: %s", strings.Join(missing, ", "))
	}
	return nil
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

// startRunLogs creates the directory for this run's task logs, or returns ""
// with a warning when it cannot, in which case tasks run without logs.
func startRunLogs(now time.Time, w *warnings) string {
	root, err := runlog.Root()
	if err != nil {
		w.add(fmt.Sprintf("task logs are not kept: %v", err))
		return ""
	}
	dir, err := runlog.Start(root, now, runlog.Keep)
	if err != nil {
		w.add(err.Error())
	}
	return dir
}

// warnings collects problems that do not fail the run, printing each to
// out as it happens.
type warnings struct {
	out io.Writer

	mu   sync.Mutex
	msgs []string
}

func (w *warnings) add(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.msgs = append(w.msgs, msg)
	_, _ = fmt.Fprintf(w.out, "nix-tasks: warning: %s\n", msg)
}

func (w *warnings) list() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.msgs)
}
