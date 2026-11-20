package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// Nix is what the runner needs from Nix. *nix.Evaluator satisfies it.
type Nix interface {
	ShellDrvPaths(ctx context.Context, system string, tasks []string) (map[string]string, error)
	PrintDevEnvCmd(ctx context.Context, drvPath, profile string) *exec.Cmd
	BuildCmd(ctx context.Context, attr string) *exec.Cmd
}

// Options configures task execution
type Options struct {
	// Jobs bounds how many tasks run at once (default runtime.GOMAXPROCS(0),
	// the number of CPUs this process may use).
	Jobs int
	// ContinueOnError keeps running independent tasks after a failure
	// instead of cancelling the rest of the run.
	ContinueOnError bool

	Force      bool   // Bypass cache (re-run even if cached)
	NoCache    bool   // Don't read or write cache
	ProjectKey string // Cache key for project

	// Reporter receives task events and output; nil discards them.
	Reporter Reporter

	// FlakeRoot is the directory tasks run in and relative workingDirs and
	// .nix-tasks/ resolve against. Empty means the current directory.
	FlakeRoot string

	// DevEnvDir caches task shell environments. Empty means the devenv
	// directory under the nix-tasks cache directory.
	DevEnvDir string
}

// Executor runs a task and its dependencies
type Executor struct {
	nix      Nix
	config   *config.Config
	options  Options
	cache    *cache.Store
	reporter Reporter
	devEnvs  *devEnvs
}

// NewExecutor creates a task executor
func NewExecutor(n Nix, cfg *config.Config, opts Options) *Executor {
	if opts.Jobs <= 0 {
		opts.Jobs = runtime.GOMAXPROCS(0)
	}
	if opts.DevEnvDir == "" {
		opts.DevEnvDir = filepath.Join(cache.Dir(), "devenv")
	}
	var store *cache.Store
	if !opts.NoCache && opts.ProjectKey != "" {
		store = cache.NewStore(opts.ProjectKey)
	}
	var reporter Reporter = discardReporter{}
	if opts.Reporter != nil {
		reporter = opts.Reporter
	}
	return &Executor{
		nix:      n,
		config:   cfg,
		options:  opts,
		cache:    store,
		reporter: reporter,
	}
}

// execTask executes a single task (dispatches to type-specific implementation)
func (e *Executor) execTask(ctx context.Context, name string, task config.Task) error {
	e.reporter.TaskStarted(name)

	// Default to shell type if not specified
	taskType := task.Type
	if taskType == "" {
		taskType = config.TaskTypeShell
	}

	switch taskType {
	case config.TaskTypeShell:
		return e.runShellTask(ctx, name, task)
	case config.TaskTypeBuild:
		return e.runBuildTask(ctx, name)
	default:
		return fmt.Errorf("unknown task type: %s", taskType)
	}
}

// taskDir returns the directory a task runs in.
func (e *Executor) taskDir(task config.Task) string {
	if filepath.IsAbs(task.WorkingDir) {
		return task.WorkingDir
	}
	return filepath.Join(e.options.FlakeRoot, task.WorkingDir)
}

// taskScript returns the script a shell task runs, empty when it has none.
func taskScript(task config.Task) string {
	if task.Script != "" {
		return task.Script
	}
	return strings.Join(task.Commands, "\n")
}

// runShellTask runs a shell task's script with bash in the environment of
// the task's shell. A task with nothing to run, such as one that only groups
// other tasks, succeeds without realising its shell.
func (e *Executor) runShellTask(ctx context.Context, name string, task config.Task) error {
	script := taskScript(task)
	if script == "" {
		return nil
	}

	stdout, stderr := e.reporter.TaskOutput(name)

	drvPath := task.ShellDrvPath
	if drvPath == "" {
		var err error
		if drvPath, err = e.devEnvs.drvPath(ctx, name); err != nil {
			return err
		}
	}
	devEnv, err := e.devEnvs.env(ctx, drvPath, stderr)
	if err != nil {
		return err
	}

	dir := e.taskDir(task)
	env := withTaskEnv(devEnv.Environ(os.Environ(), dir), task.Env)

	bash, err := lookPath("bash", envValue(env, "PATH"))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bash, "-e", "-o", "pipefail", "-c", script)
	isolateProcessGroup(cmd)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Dir = dir
	cmd.Env = env

	slog.Debug("starting task shell", "task", name, "drv", drvPath)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return err
	}
	slog.Debug("task shell exited", "task", name, "duration", time.Since(start))

	return nil
}

// withTaskEnv applies a task's env over env. A task's PATH is prepended, so
// adding a directory does not hide the shell's tools.
func withTaskEnv(env []string, taskEnv map[string]string) []string {
	for k, v := range taskEnv {
		if k == "PATH" {
			if current := envValue(env, "PATH"); current != "" {
				v += ":" + current
			}
		}
		env = append(env, k+"="+v)
	}
	return env
}

// envValue returns the last value of key in env, which is the one a process
// started with env sees.
func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// runBuildTask builds the task's nixTasksBuilds flake attribute and links
// its outputs
func (e *Executor) runBuildTask(ctx context.Context, name string) error {
	attr := nix.NixTasksBuildsAttr(nix.CurrentSystem(), name)
	cmd := e.nix.BuildCmd(ctx, attr)
	isolateProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	_, taskStderr := e.reporter.TaskOutput(name)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(taskStderr, &stderr)

	slog.Debug("starting nix build", "task", name, "attr", attr)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "does not provide attribute") {
			return fmt.Errorf("nix build failed: %w (does the flake expose nixTasksBuilds? generate its outputs with nix-tasks.lib.mkFlakeOutputs)", err)
		}
		return fmt.Errorf("nix build failed: %w", err)
	}
	slog.Debug("nix build exited", "task", name, "duration", time.Since(start))

	outputs, err := nix.ParseBuildOutputs(stdout.Bytes())
	if err != nil {
		return err
	}

	if err := e.linkBuildOutputs(name, outputs); err != nil {
		return fmt.Errorf("failed to link outputs: %w", err)
	}

	return nil
}

// linkBuildOutputs creates symlinks in .nix-tasks/<taskname>/<output>
func (e *Executor) linkBuildOutputs(name string, outputs map[string]string) error {
	taskDir := filepath.Join(e.options.FlakeRoot, ".nix-tasks", name)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return err
	}

	for outputName, outputPath := range outputs {
		linkPath := filepath.Join(taskDir, outputName)

		if err := os.Remove(linkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to replace %s: %w", linkPath, err)
		}

		if err := os.Symlink(outputPath, linkPath); err != nil {
			return fmt.Errorf("failed to link %s: %w", outputName, err)
		}

		slog.Debug("linked output", "task", name, "output", outputName, "path", outputPath, "link", linkPath)
	}

	return nil
}
