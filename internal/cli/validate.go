package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ValidateCmd validates the configuration
type ValidateCmd struct{}

// Run executes the validate command
func (c *ValidateCmd) Run(globals *Globals) error {
	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	// Configuration is already validated during Load, but we can do additional checks
	err = config.Validate(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("%s Configuration is valid\n", ui.Success("✓"))
	fmt.Printf("  Tasks: %d\n", len(cfg.Tasks))
	fmt.Printf("  Packages: %d\n", len(cfg.Packages))
	fmt.Printf("  Dev Shells: %d\n", len(cfg.DevShells))

	return nil
}
