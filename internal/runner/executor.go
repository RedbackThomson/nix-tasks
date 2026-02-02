package runner

import (
	"context"
	"fmt"
	"os"

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
	nix     *nix.Evaluator
	config  *config.Config
	options ExecutorOptions
	printer *ui.Printer
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

// RunTask executes a single task
func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
	e.printer.TaskStarted(name)

	// Generate the script to run
	script := GenerateScript(task)

	// Run inside nix develop with the task's shell
	// Try system-specific path first, then fall back to non-system-specific
	system := nix.CurrentSystem()
	shellAttr := fmt.Sprintf("nixTasksShells.%s.%s", system, name)
	cmd := e.nix.DevelopCmd(ctx, shellAttr, []string{"bash", "-e", "-c", script})

	// Configure output
	if e.options.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// Buffer output, only show on error
		cmd.Stdout = e.printer.TaskBuffer(name)
		cmd.Stderr = e.printer.TaskBuffer(name)
	}

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
	err := cmd.Run()
	if err != nil {
		e.printer.TaskFailed(name, err)
		return fmt.Errorf("task '%s' failed: %w", name, err)
	}

	e.printer.TaskSucceeded(name)
	return nil
}
