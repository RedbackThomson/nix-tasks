package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ExecutorOptions configures task execution
type ExecutorOptions struct {
	Verbose    bool
	Debug      bool
	Force      bool   // Bypass cache (re-run even if cached)
	NoCache    bool   // Don't read or write cache
	ProjectKey string // Cache key for project
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
// In verbose/streaming mode, output is prefixed with the task name.
// Otherwise, output is buffered and only shown on failure.
func (e *Executor) taskOutput(name string) (stdout, stderr io.Writer) {
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
	e.printer.TaskStarted(name)

	// Default to shell type if not specified
	taskType := task.Type
	if taskType == "" {
		taskType = config.TaskTypeShell
	}

	var err error
	switch taskType {
	case config.TaskTypeShell:
		err = e.runShellTask(ctx, name, task)
	case config.TaskTypeBuild:
		err = e.runBuildTask(ctx, name, task)
	default:
		err = fmt.Errorf("unknown task type: %s", taskType)
	}

	if err != nil {
		e.printer.TaskFailed(name, err)
		return err
	}

	e.printer.TaskSucceeded(name)
	return nil
}

// runShellTask executes a shell task in nix develop
func (e *Executor) runShellTask(ctx context.Context, name string, task config.Task) error {
	// Generate the script to run
	script := GenerateScript(task)

	// Run inside nix develop with the task's shell
	system := nix.CurrentSystem()
	shellAttr := fmt.Sprintf("nixTasksShells.%s.%s", system, name)
	cmd := e.nix.DevelopCmd(ctx, shellAttr, []string{"bash", "-e", "-c", script})

	// Configure output
	cmd.Stdout, cmd.Stderr = e.taskOutput(name)

	// Set working directory if specified
	if task.WorkingDir != "" {
		cmd.Dir = task.WorkingDir
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("task '%s' failed: %w", name, err)
	}

	return nil
}

// runBuildTask builds a Nix derivation and links outputs
func (e *Executor) runBuildTask(ctx context.Context, name string, task config.Task) error {
	if task.DrvPath == "" {
		return fmt.Errorf("build task %s missing drvPath", name)
	}

	if len(task.Outputs) == 0 {
		return fmt.Errorf("build task %s has no outputs", name)
	}

	// Build the derivation
	cmd := e.nix.BuildCmd(ctx, task.DrvPath)

	// Configure output
	cmd.Stdout, cmd.Stderr = e.taskOutput(name)

	// Execute nix build
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix build failed: %w", err)
	}

	// Link outputs to workspace
	if err := e.linkBuildOutputs(name, task); err != nil {
		return fmt.Errorf("failed to link outputs: %w", err)
	}

	return nil
}

// linkBuildOutputs creates symlinks in .nix-tasks/<taskname>/<output>
func (e *Executor) linkBuildOutputs(name string, task config.Task) error {
	taskDir := filepath.Join(".nix-tasks", name)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return err
	}

	// Link each output
	for outputName, outputPath := range task.Outputs {
		linkPath := filepath.Join(taskDir, outputName)

		// Remove existing link/file
		os.Remove(linkPath)

		// Create symlink to store path
		if err := os.Symlink(outputPath, linkPath); err != nil {
			return fmt.Errorf("failed to link %s: %w", outputName, err)
		}

		slog.Debug("linked output", "task", name, "output", outputName, "path", outputPath, "link", linkPath)
	}

	return nil
}
