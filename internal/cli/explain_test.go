package cli

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

func TestWriteExplain(t *testing.T) {
	ui.SetColor(false)
	cached := &config.TaskCache{Inputs: []string{"a"}, Outputs: []string{}}
	cfg := &config.Config{Tasks: map[string]config.Task{
		"tools": {}, "generate": {Cache: cached}, "mocks": {Cache: cached}, "crds": {Cache: cached},
		"fresh": {Cache: cached}, "down": {Cache: cached}, "bad": {Cache: cached}, "later": {Cache: cached},
	}}
	results := []runner.TaskResult{
		{Name: "tools", State: runner.Succeeded},
		{Name: "generate", State: runner.Succeeded, Cache: &runner.CacheOutcome{Stored: true, Miss: &cache.Explanation{
			Inputs:   cache.FileChanges{Modified: []string{"apis/v1/types.go"}, Added: []string{"apis/v1/new.go"}},
			Upstream: []string{"tools"},
		}}},
		{Name: "mocks", State: runner.Succeeded, Cached: true, Saved: 12400 * time.Millisecond, Cache: &runner.CacheOutcome{}},
		{Name: "crds", State: runner.Succeeded, Cache: &runner.CacheOutcome{Stored: true, Miss: &cache.Explanation{
			OutputsChanged: true, Outputs: cache.FileChanges{Modified: []string{"package/crds/foo.yaml"}},
		}}},
		{Name: "fresh", State: runner.Succeeded, Cache: &runner.CacheOutcome{
			Miss: &cache.Explanation{NoPrevious: true},
		}, Warnings: []string{"not cached: inputs changed while it ran: a"}},
		{Name: "down", State: runner.Succeeded, Cache: &runner.CacheOutcome{Uncached: "depends on uncached tools"}},
		{Name: "bad", State: runner.Failed, Error: errors.New("x"), Cache: &runner.CacheOutcome{Forced: true}},
		{Name: "later", State: runner.Skipped},
	}

	var buf bytes.Buffer
	writeExplain(&buf, cfg, results, "")
	want := `
generate: miss
  inputs changed: 1 modified, 1 added
    M apis/v1/types.go
    A apis/v1/new.go
  upstream changed: tools
mocks: hit (saved 12s)
crds: miss — outputs changed since last run
    M package/crds/foo.yaml
fresh: miss — no previous entry
  not stored: inputs changed while it ran: a
down: not cached — depends on uncached tools
bad: forced by --force
  not stored: task failed
later: not run (skipped)
`
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}

	buf.Reset()
	writeExplain(&buf, cfg, []runner.TaskResult{{Name: "generate", State: runner.Succeeded}}, "--no-cache")
	if buf.String() != "\ngenerate: not cached — --no-cache\n" {
		t.Errorf("disabled: %q", buf.String())
	}
}
