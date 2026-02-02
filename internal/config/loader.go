package config

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// Load evaluates the Nix configuration and returns the parsed config
func Load(ctx context.Context, eval *nix.Evaluator) (*Config, error) {
	var cfg Config

	// Try nixTasksConfig first (flake output)
	err := eval.Eval(ctx, "nixTasksConfig", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate nixTasksConfig: %w", err)
	}

	// Initialize empty maps if nil
	if cfg.Packages == nil {
		cfg.Packages = make(map[string]string)
	}
	if cfg.Tasks == nil {
		cfg.Tasks = make(map[string]Task)
	}
	if cfg.DevShells == nil {
		cfg.DevShells = make(map[string]Shell)
	}

	// Validate the loaded config
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
