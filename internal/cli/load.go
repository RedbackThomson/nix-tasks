package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// loadConfig evaluates the configuration of the flake selected by -f.
func loadConfig(ctx context.Context, globals *Globals) (*config.Config, *nix.Evaluator, error) {
	eval := nix.NewEvaluator(globals.Flake)
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return nil, eval, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, eval, nil
}
