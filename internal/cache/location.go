package cache

import (
	"os"
	"path/filepath"
)

// Dir returns the cache directory for nix-tasks
func Dir() string {
	// Check environment variable first
	if dir := os.Getenv("NIX_TASKS_CACHE_DIR"); dir != "" {
		return dir
	}

	// Use XDG cache directory
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "nix-tasks")
	}

	// Fall back to ~/.cache/nix-tasks
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "nix-tasks")
}
