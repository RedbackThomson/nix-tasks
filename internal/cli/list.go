package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ListCmd lists available tasks
type ListCmd struct{}

// Run executes the list command
func (c *ListCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	printer := ui.NewPrinter()
	printer.PrintTaskList(cfg.Tasks)
	printer.PrintShellList(cfg.DevShells)

	return nil
}
