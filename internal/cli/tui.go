package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/redbackthomson/nix-tasks/internal/tui"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// TUICmd launches the interactive TUI
type TUICmd struct{}

// Run executes the TUI command
func (c *TUICmd) Run(globals *Globals) error {
	// Check if we're in a TTY
	if !ui.IsTerminal(os.Stdout) {
		return fmt.Errorf("TUI requires an interactive terminal")
	}

	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	return tui.Run(cfg, globals.Flake)
}
