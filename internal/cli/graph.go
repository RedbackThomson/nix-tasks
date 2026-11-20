package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// GraphCmd shows the execution graph for a task
type GraphCmd struct {
	Task string `arg:"" completion-predictor:"task" help:"Task name to show execution graph for"`
}

// Run executes the graph command
func (c *GraphCmd) Run(globals *Globals) error {
	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	if _, ok := cfg.Tasks[c.Task]; !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}

	lines, err := ui.GraphTree(graph, c.Task)
	if err != nil {
		return fmt.Errorf("failed to compute execution order: %w", err)
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}
