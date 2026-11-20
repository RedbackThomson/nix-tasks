package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ListCmd lists available tasks
type ListCmd struct {
	// Names and Shells give completion scripts a stable format to parse.
	Names  bool `hidden:"" help:"Print only task names, one per line"`
	Shells bool `hidden:"" help:"With --names, print dev shell names instead"`
}

// Run executes the list command
func (c *ListCmd) Run(globals *Globals) error {
	if c.Shells && !c.Names {
		return fmt.Errorf("--shells requires --names")
	}

	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	if c.Names {
		printNames(os.Stdout, cfg, c.Shells)
		return nil
	}

	printer := ui.NewPrinter()
	printer.PrintTaskList(cfg.Tasks)
	printer.PrintShellList(cfg.DevShells)

	return nil
}

// printNames writes the sorted task names, or dev shell names when shells is
// set, one per line.
func printNames(w io.Writer, cfg *config.Config, shells bool) {
	var names []string
	if shells {
		names = slices.Sorted(maps.Keys(cfg.DevShells))
	} else {
		names = slices.Sorted(maps.Keys(cfg.Tasks))
	}
	for _, name := range names {
		_, _ = fmt.Fprintln(w, name)
	}
}
