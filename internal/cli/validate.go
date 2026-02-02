package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ValidateCmd validates the configuration
type ValidateCmd struct{}

// Run executes the validate command
func (c *ValidateCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Configuration is already validated during Load, but we can do additional checks
	err = config.Validate(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("%s Configuration is valid\n", ui.Green("✓"))
	fmt.Printf("  Tasks: %d\n", len(cfg.Tasks))
	fmt.Printf("  Packages: %d\n", len(cfg.Packages))
	fmt.Printf("  Dev Shells: %d\n", len(cfg.DevShells))

	return nil
}
