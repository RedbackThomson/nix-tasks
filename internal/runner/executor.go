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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ExecutorOptions configures task execution
type ExecutorOptions struct {
	Verbose    bool
	Debug      bool
	Force      bool                // Bypass cache (re-run even if cached)
	NoCache    bool                // Don't read or write cache
	ProjectKey string              // Cache key for project
	RawTarget  string              // If non-empty, pipe this task's output directly (no decoration)
	Progress   *ui.ProgressDisplay // Live progress display (nil unless Progress mode)

	// FlakeRoot is the directory tasks run in and relative workingDirs and
	// .nix-tasks/ resolve against. Empty means the current directory.
	FlakeRoot string
}

// Executor runs tasks
type Executor struct {
	nix      *nix.Evaluator
	config   *config.Config
	options  ExecutorOptions
	printer  *ui.Printer
	streamMu sync.Mutex // guards streaming output across tasks
}

// NewExecutor creates a new task executor
func NewExecutor(eval *nix.Evaluator, cfg *config.Config, opts ExecutorOptions) *Executor {
	eval.SetDebug(opts.Debug)
	return &Executor{
		nix:     eval,
		config:  cfg,
		options: opts,
		printer: ui.NewPrinter(),
	}
}

// taskOutput returns the stdout and stderr writers for a task.
// In raw mode, the target task's output pipes directly to os.Stdout/os.Stderr.
// In verbose/streaming mode, output is prefixed with the task name.
// Otherwise, output is buffered and only shown on failure.
func (e *Executor) taskOutput(name string) (stdout, stderr io.Writer) {
	if e.options.RawTarget == name {
		return os.Stdout, os.Stderr
	}
	if e.options.Progress != nil {
		w := e.options.Progress.Writer(name)
		return w, w
	}
	if e.options.Verbose {
		pw := &prefixWriter{
			prefix: fmt.Sprintf("[%s] ", name),
			out:    os.Stdout,
			mu:     &e.streamMu,
		}
		return pw, pw
	}
	buf := e.printer.TaskBuffer(name)
	return buf, buf
}

// RunTask executes a single task (dispatches to type-specific implementation)
func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
	if e.options.Progress != nil {
		e.options.Progress.TaskStarted(name)
	} else {
		e.printer.TaskStarted(name)
	}

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

// runShellTask executes a shell task in nix develop
func (e *Executor) runShellTask(ctx context.Context, name string, task config.Task) error {
	// Generate the script to run
	script := GenerateScript(task)

	// Run inside nix develop with the task's shell
	system := nix.CurrentSystem()
	shellAttr := nix.NixTasksShellsAttr(system, name)
	cmd := e.nix.DevelopCmd(ctx, shellAttr, []string{"bash", "-e", "-c", script})

	// Configure output
	cmd.Stdout, cmd.Stderr = e.taskOutput(name)

	cmd.Dir = e.taskDir(task)

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute
	slog.Debug("starting nix develop shell", "task", name, "shell", shellAttr)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("task '%s' failed: %w", name, err)
	}
	slog.Debug("nix develop shell exited", "task", name, "duration", time.Since(start))

	return nil
}

// runBuildTask builds the task's nixTasksBuilds flake attribute and links
// its outputs
func (e *Executor) runBuildTask(ctx context.Context, name string) error {
	attr := nix.NixTasksBuildsAttr(nix.CurrentSystem(), name)
	cmd := e.nix.BuildCmd(ctx, attr)

	var stdout, stderr bytes.Buffer
	_, taskStderr := e.taskOutput(name)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(taskStderr, &stderr)

	slog.Debug("starting nix build", "task", name, "attr", attr)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "does not provide attribute") {
			return fmt.Errorf("nix build failed: %w (does the flake expose nixTasksBuilds? add `nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);` to its outputs)", err)
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
