package config

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// initializeMaps ensures all config maps are initialized to non-nil values
func initializeMaps(cfg *Config) {
	if cfg.Packages == nil {
		cfg.Packages = make(map[string]string)
	}
	if cfg.Tasks == nil {
		cfg.Tasks = make(map[string]Task)
	}
	if cfg.DevShells == nil {
		cfg.DevShells = make(map[string]Shell)
	}
}

// Load evaluates the Nix configuration and returns the parsed config
func Load(ctx context.Context, eval *nix.Evaluator) (*Config, error) {
	var cfg Config

	system := nix.CurrentSystem()

	// Try system-specific nixTasksConfig first (e.g., nixTasksConfig.aarch64-darwin)
	err := eval.Eval(ctx, fmt.Sprintf("nixTasksConfig.%s", system), &cfg)
	if err != nil {
		// Fall back to non-system-specific nixTasksConfig
		err = eval.Eval(ctx, "nixTasksConfig", &cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate nixTasksConfig: %w", err)
		}
	}

	// Initialize empty maps if nil
	initializeMaps(&cfg)

	// Validate the loaded config
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
