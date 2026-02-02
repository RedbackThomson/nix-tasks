package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// RunCmd runs a task
type RunCmd struct {
	Task string `arg:"" help:"Task name to run"`
}

// Run executes the run command
func (c *RunCmd) Run(globals *Globals) error {
	ctx := context.Background()

	// Initialize Nix evaluator
	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	// Load configuration
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate task exists
	task, ok := cfg.Tasks[c.Task]
	if !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Execute task
	exec := runner.NewExecutor(eval, cfg, runner.ExecutorOptions{
		Verbose: globals.Verbose,
		Debug:   globals.Debug,
	})

	return exec.RunTask(ctx, c.Task, task)
}
