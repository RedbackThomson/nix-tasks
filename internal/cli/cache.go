package cli

import (
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/cache"
)

// CacheCmd contains cache management subcommands
type CacheCmd struct {
	Clean CacheCleanCmd `cmd:"" help:"Clear the cache for this project"`
	Stats CacheStatsCmd `cmd:"" help:"Show cache statistics"`
}

// CacheCleanCmd clears the cache
type CacheCleanCmd struct{}

// Run executes the cache clean command
func (c *CacheCleanCmd) Run(globals *Globals) error {
	projectKey := cache.ProjectKey(globals.Flake)
	store := cache.NewStore(projectKey)

	if err := store.Clear(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared")
	return nil
}

// CacheStatsCmd shows cache statistics
type CacheStatsCmd struct{}

// Run executes the cache stats command
func (c *CacheStatsCmd) Run(globals *Globals) error {
	projectKey := cache.ProjectKey(globals.Flake)
	store := cache.NewStore(projectKey)

	stats, err := store.Stats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	fmt.Printf("Cache Statistics\n")
	fmt.Printf("  Location:    %s\n", cache.Dir())
	fmt.Printf("  Project Key: %s\n", projectKey)
	fmt.Printf("  Entries:     %d\n", stats.EntryCount)

	if stats.EntryCount > 0 {
		fmt.Printf("  Total Size:  %s\n", formatBytes(stats.TotalSize))
		fmt.Printf("  Oldest:      %s\n", stats.OldestTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Newest:      %s\n", stats.NewestTime.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// formatBytes formats a byte count in human-readable form
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
