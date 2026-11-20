package cli

import (
	"context"
	"os"

	"github.com/redbackthomson/nix-tasks/internal/tui"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// TUICmd launches the interactive TUI
type TUICmd struct{}

// Run executes the TUI command
func (c *TUICmd) Run(globals *Globals) error {
	// Scripts and pipes that run plain nix-tasks get something they can read.
	if !ui.IsTerminal(os.Stdout) {
		return (&ListCmd{}).Run(globals)
	}

	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	return tui.Run(cfg, globals.Flake)
}
