package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// writeExplain prints what the cache did for each task in the run that
// declares a cache. disabled is why the cache was off for the whole run, or
// "".
func writeExplain(out io.Writer, cfg *config.Config, results []runner.TaskResult, disabled string) {
	var b strings.Builder
	w := &b
	first := true
	for _, r := range results {
		if cfg.Tasks[r.Name].Cache == nil {
			continue
		}
		if first {
			w.WriteString("\n")
			first = false
		}
		name := r.Name + ":"
		c := r.Cache
		switch {
		case c == nil && disabled != "":
			fmt.Fprintf(w, "%s not cached — %s\n", name, disabled)
		case c == nil:
			fmt.Fprintf(w, "%s not run (%s)\n", name, r.State)
		case r.Cached && r.Saved > 0:
			fmt.Fprintf(w, "%s hit (saved %s)\n", name, ui.ShortDuration(r.Saved))
		case r.Cached:
			fmt.Fprintf(w, "%s hit\n", name)
		case c.Uncached != "":
			fmt.Fprintf(w, "%s not cached — %s\n", name, c.Uncached)
		case c.Forced:
			fmt.Fprintf(w, "%s forced by --force\n", name)
		case c.Miss == nil:
			fmt.Fprintf(w, "%s not run (%s)\n", name, r.State)
		case c.Miss.NoPrevious || c.Miss.OutputsChanged:
			fmt.Fprintf(w, "%s miss — %s\n", name, c.Miss.Reason())
			c.Miss.Write(w, "  ")
		default:
			fmt.Fprintf(w, "%s miss\n", name)
			c.Miss.Write(w, "  ")
		}
		if c != nil && !r.Cached && c.Uncached == "" && !c.Stored && r.State != runner.Skipped {
			fmt.Fprintf(w, "  not stored: %s\n", notStoredReason(r))
		}
	}
	_, _ = io.WriteString(out, b.String())
}

func notStoredReason(r runner.TaskResult) string {
	if r.State != runner.Succeeded {
		return "task " + r.State.String()
	}
	if len(r.Warnings) > 0 {
		return strings.TrimPrefix(r.Warnings[len(r.Warnings)-1], "not cached: ")
	}
	return "run was interrupted"
}
