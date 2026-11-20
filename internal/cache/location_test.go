package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	t.Setenv("NIX_TASKS_CACHE_DIR", "/custom/cache")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	if got := Dir(); got != "/custom/cache" {
		t.Errorf("NIX_TASKS_CACHE_DIR: Dir() = %q", got)
	}

	t.Setenv("NIX_TASKS_CACHE_DIR", "")
	if got := Dir(); got != "/xdg/cache/nix-tasks" {
		t.Errorf("XDG_CACHE_HOME: Dir() = %q", got)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	home, _ := os.UserHomeDir()
	if got := Dir(); got != filepath.Join(home, ".cache", "nix-tasks") {
		t.Errorf("fallback: Dir() = %q", got)
	}
}
