package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/tui"
)

// TUICmd launches the interactive TUI
type TUICmd struct{}

// Run executes the TUI command
func (c *TUICmd) Run(globals *Globals) error {
	// Check if we're in a TTY
	if !isTerminal() {
		return fmt.Errorf("TUI requires an interactive terminal")
	}

	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return tui.Run(cfg, globals.Flake)
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
