package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// CacheCmd contains cache management subcommands
type CacheCmd struct {
	Clean CacheCleanCmd `cmd:"" help:"Remove this repository's cache entries"`
	Stats CacheStatsCmd `cmd:"" help:"Show cache entries per task"`
}

// openStore returns the store shared by every checkout of the flake's
// repository.
func openStore(ctx context.Context, globals *Globals) (*cache.Store, error) {
	if globals.FlakeRoot == "" {
		return nil, fmt.Errorf("the task cache needs a local flake; %s is not a local path", globals.Flake)
	}
	return cache.Open(cache.Dir(), cache.ProjectKey(ctx, globals.FlakeRoot)), nil
}

// CacheCleanCmd clears the cache
type CacheCleanCmd struct {
	Task string `completion-predictor:"task" help:"Remove only this task's entries"`
}

// Run executes the cache clean command
func (c *CacheCleanCmd) Run(globals *Globals) error {
	store, err := openStore(context.Background(), globals)
	if err != nil {
		return err
	}

	if c.Task != "" {
		if err := store.CleanTask(c.Task); err != nil {
			return fmt.Errorf("failed to clear the cache of %s: %w", c.Task, err)
		}
		fmt.Printf("Cache cleared for %s\n", c.Task)
		return nil
	}

	if err := store.Clean(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	if err := cache.RemoveLegacy(cache.Dir(), globals.FlakeRoot); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	fmt.Println("Cache cleared")
	return nil
}

// CacheStatsCmd shows cache statistics
type CacheStatsCmd struct{}

// Run executes the cache stats command
func (c *CacheStatsCmd) Run(globals *Globals) error {
	store, err := openStore(context.Background(), globals)
	if err != nil {
		return err
	}
	printStats(os.Stdout, store.Dir(), store.Stats())
	return nil
}

func printStats(w io.Writer, dir string, stats []cache.TaskStats) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", ui.Label("Cache:"), dir)
	if len(stats) == 0 {
		b.WriteString("No cache entries\n")
		_, _ = io.WriteString(w, b.String())
		return
	}
	b.WriteString("\n")

	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TASK\tENTRIES\tSIZE\tLAST HIT\tLAST RUN")
	var entries int
	var size int64
	for _, s := range stats {
		lastHit := "never"
		if !s.LastHit.IsZero() {
			lastHit = s.LastHit.Local().Format(time.DateTime)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", s.Task, s.Entries, formatBytes(s.Size), lastHit, ui.ShortDuration(s.RunDuration))
		entries += s.Entries
		size += s.Size
	}
	_ = tw.Flush()
	fmt.Fprintf(&b, "\nTotal: %d tasks, %d entries, %s\n", len(stats), entries, formatBytes(size))
	_, _ = io.WriteString(w, b.String())
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
