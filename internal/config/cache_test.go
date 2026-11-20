package config_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func cachedTask(mutate func(*config.Task)) config.Task {
	t := config.Task{
		Type:         config.TaskTypeShell,
		Commands:     []string{"go generate ./..."},
		ShellDrvPath: "/nix/store/x-nix-shell.drv",
		Cache:        &config.TaskCache{Inputs: []string{"apis/**/*.go"}, Outputs: []string{}},
	}
	if mutate != nil {
		mutate(&t)
	}
	return t
}

func TestValidate_Cache(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Task)
		wantErr string
	}{
		{"valid", nil, ""},
		{"build task", func(t *config.Task) { t.Type = config.TaskTypeBuild }, "build tasks cannot set cache"},
		{"no commands", func(t *config.Task) { t.Commands = nil }, "no commands or script"},
		{"script is enough", func(t *config.Task) { t.Commands = nil; t.Script = "gen" }, ""},
		{"empty inputs", func(t *config.Task) { t.Cache.Inputs = nil }, "at least one pattern"},
		{"missing outputs", func(t *config.Task) { t.Cache.Outputs = nil }, "cache.outputs is required"},
		{"absolute pattern", func(t *config.Task) { t.Cache.Inputs = []string{"/etc/passwd"} }, "is absolute"},
		{"parent pattern", func(t *config.Task) { t.Cache.Outputs = []string{"../x"} }, "contains .."},
		{"excluded parent pattern", func(t *config.Task) { t.Cache.Inputs = []string{"a", "!a/../b"} }, "contains .."},
		{"invalid glob", func(t *config.Task) { t.Cache.Inputs = []string{"a/[b"} }, "not a valid glob"},
		{"invalid env name", func(t *config.Task) { t.Cache.Env = []string{"GO FLAGS"} }, "not a valid variable name"},
		{"missing shellDrvPath", func(t *config.Task) { t.ShellDrvPath = "" }, "no shellDrvPath"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Tasks: map[string]config.Task{"gen": cachedTask(tt.mutate)}}
			err := config.Validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var vErr *config.ValidationError
			if !errors.As(err, &vErr) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCacheWarnings(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{
		"tools": {Commands: []string{"install"}},
		"mid": cachedTask(func(t *config.Task) {
			t.Depends = []string{"tools"}
		}),
		"gen": cachedTask(func(t *config.Task) {
			t.After = []string{"mid"}
			t.Env = map[string]string{"GOFLAGS": "-x"}
			t.Cache.Env = []string{"GOFLAGS"}
			t.Cache.Inputs = []string{"**/*.go", "apis/"}
		}),
	}}
	got := config.CacheWarnings(cfg)
	want := []string{
		"task 'gen': cache.env names GOFLAGS",
		`task 'gen': cache.inputs pattern "**/*.go" walks the whole flake`,
		`task 'gen': cache.inputs pattern "apis/" ends in /`,
		"gen has cache enabled but always runs: it depends on tools, which is not cached",
		"mid has cache enabled but always runs: it depends on tools, which is not cached",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d warnings:\n%s", len(got), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("warning %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
}

func TestUncachedUpstreams(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{
		"a":    {Commands: []string{"a"}},
		"b":    cachedTask(func(t *config.Task) { t.Depends = []string{"a"} }),
		"c":    {Commands: []string{"c"}},
		"gen":  cachedTask(func(t *config.Task) { t.Depends = []string{"b"}; t.After = []string{"c"} }),
		"ok":   cachedTask(func(t *config.Task) { t.Depends = []string{"ok2", "bin"} }),
		"bin":  {Type: config.TaskTypeBuild, Depends: []string{"ok2"}},
		"via":  cachedTask(func(t *config.Task) { t.Depends = []string{"bin2"} }),
		"bin2": {Type: config.TaskTypeBuild, Depends: []string{"a"}},
		"ok2":  cachedTask(nil),
	}}
	if got := config.UncachedUpstreams(cfg, "gen"); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("gen: %v", got)
	}
	if got := config.UncachedUpstreams(cfg, "ok"); len(got) != 0 {
		t.Errorf("a build task upstream is not uncached: %v", got)
	}
	if got := config.UncachedUpstreams(cfg, "via"); !slices.Equal(got, []string{"a"}) {
		t.Errorf("an uncached shell task behind a build task still counts: %v", got)
	}
	if w := config.AlwaysRunsWarning(cfg, "gen"); w != "gen has cache enabled but always runs: it depends on a, c, which are not cached" {
		t.Errorf("warning %q", w)
	}
}
