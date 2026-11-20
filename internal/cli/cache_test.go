package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

func TestPrintCacheSection(t *testing.T) {
	ui.SetColor(false)
	cfg := &config.Config{
		CacheSettings: config.CacheSettings{MaxEntriesPerTask: 16},
		Tasks: map[string]config.Task{
			"tools": {Commands: []string{"install"}},
			"gen": {Commands: []string{"gen"}, Depends: []string{"tools"}, Cache: &config.TaskCache{
				Inputs: []string{"apis/**/*.go", "!apis/**/testdata/**"}, Outputs: []string{}, Env: []string{"GOFLAGS"},
			}},
		},
	}
	t.Setenv(cache.MaxEntriesEnv, "")

	var buf strings.Builder
	printCacheSection(&buf, cfg, "gen")
	want := `Cache:
  Inputs:
    - apis/**/*.go
    - !apis/**/testdata/**
  Outputs: none
  Env:
    - GOFLAGS
  Max entries: 16
  warning: gen has cache enabled but always runs: it depends on tools, which is not cached

`
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}

	buf.Reset()
	printCacheSection(&buf, cfg, "tools")
	if buf.String() != "Cache: disabled\n\n" {
		t.Errorf("uncached task: %q", buf.String())
	}
}

func TestPrintStats(t *testing.T) {
	ui.SetColor(false)
	var buf strings.Builder
	printStats(&buf, "/cache/v2/abc", nil)
	if buf.String() != "Cache: /cache/v2/abc\nNo cache entries\n" {
		t.Errorf("empty: %q", buf.String())
	}

	buf.Reset()
	printStats(&buf, "/cache/v2/abc", []cache.TaskStats{
		{Task: "gen", Entries: 3, Size: 2048, RunDuration: 41 * time.Second},
		{Task: "mocks", Entries: 1, Size: 100, RunDuration: 1200 * time.Millisecond, LastHit: time.Now()},
	})
	out := buf.String()
	for _, want := range []string{"TASK", "gen", "2.0 KB", "never", "41s", "mocks", "1.2s", "Total: 2 tasks, 4 entries, 2.1 KB"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
