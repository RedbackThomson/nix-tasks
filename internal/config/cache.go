package config

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateCache returns the errors in every task's cache declaration. The
// Nix library checks most of these too; checking again here covers
// configurations produced by an older library or written by hand.
func validateCache(cfg *Config) []string {
	var errs []string
	for _, name := range sortedKeys(cfg.Tasks) {
		task := cfg.Tasks[name]
		c := task.Cache
		if c == nil {
			continue
		}
		fail := func(format string, args ...any) {
			errs = append(errs, fmt.Sprintf("task '%s': ", name)+fmt.Sprintf(format, args...))
		}
		if task.Type == TaskTypeBuild {
			fail("build tasks cannot set cache")
			continue
		}
		if len(task.Commands) == 0 && task.Script == "" {
			fail("sets cache but has no commands or script to cache")
		}
		if len(c.Inputs) == 0 {
			fail("cache.inputs must list at least one pattern")
		}
		if c.Outputs == nil {
			fail("cache.outputs is required; use [ ] for a task that writes no files")
		}
		if c.MaxEntries < 0 {
			fail("cache.maxEntries must be positive")
		}
		for _, f := range patternFields(c) {
			for _, p := range f.patterns {
				if err := checkPattern(p); err != "" {
					fail("cache.%s pattern %q %s", f.name, p, err)
				}
			}
		}
		for _, v := range c.Env {
			if !envName.MatchString(v) {
				fail("cache.env entry %q is not a valid variable name", v)
			}
		}
		if task.ShellDrvPath == "" {
			fail("sets cache but the configuration has no shellDrvPath for it; update the flake's nix-tasks input")
		}
	}
	return errs
}

type patternField struct {
	name     string
	patterns []string
}

func patternFields(c *TaskCache) []patternField {
	return []patternField{{"inputs", c.Inputs}, {"outputs", c.Outputs}}
}

func checkPattern(p string) string {
	p = strings.TrimPrefix(p, "!")
	switch {
	case p == "":
		return "is empty"
	case path.IsAbs(p):
		return "is absolute; patterns are relative to the flake root"
	case slices.Contains(strings.Split(p, "/"), ".."):
		return "contains ..; patterns cannot leave the flake root"
	case !doublestar.ValidatePattern(p):
		return "is not a valid glob"
	}
	return ""
}

// CacheWarnings returns problems in cache declarations that do not stop a
// task from running, sorted by task name.
func CacheWarnings(cfg *Config) []string {
	var warnings []string
	for _, name := range sortedKeys(cfg.Tasks) {
		task := cfg.Tasks[name]
		c := task.Cache
		if c == nil || task.Type == TaskTypeBuild {
			continue
		}
		warn := func(format string, args ...any) {
			warnings = append(warnings, fmt.Sprintf("task '%s': ", name)+fmt.Sprintf(format, args...))
		}
		for _, v := range c.Env {
			if _, ok := task.Env[v]; ok {
				warn("cache.env names %s, which the task's env already sets; its value is hashed as part of the definition", v)
			}
		}
		for _, p := range c.Inputs {
			if strings.HasPrefix(p, "**") {
				warn("cache.inputs pattern %q walks the whole flake on every run; start it with a directory", p)
			}
		}
		for _, f := range patternFields(c) {
			for _, p := range f.patterns {
				if strings.HasSuffix(p, "/") {
					warn("cache.%s pattern %q ends in /; directories contribute nothing, use %q", f.name, p, p+"**")
				}
			}
		}
		if w := AlwaysRunsWarning(cfg, name); w != "" {
			warnings = append(warnings, w)
		}
	}
	return warnings
}

// Upstream returns every task transitively upstream of name through depends
// and after, sorted.
func Upstream(cfg *Config, name string) []string {
	seen := map[string]bool{name: true}
	queue := []string{name}
	var up []string
	for len(queue) > 0 {
		task := cfg.Tasks[queue[0]]
		queue = queue[1:]
		for _, dep := range slices.Concat(task.Depends, task.After) {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			queue = append(queue, dep)
			up = append(up, dep)
		}
	}
	slices.Sort(up)
	return up
}

// UncachedUpstreams returns the shell tasks, transitively upstream of name
// through depends and after, that are not cached. A cached task with any of
// them always runs: they can change anything without the cache seeing it.
// Build tasks are not among them: their derivation path stands in for their
// key.
func UncachedUpstreams(cfg *Config, name string) []string {
	var uncached []string
	for _, up := range Upstream(cfg, name) {
		if t, ok := cfg.Tasks[up]; ok && t.Cache == nil && t.Type != TaskTypeBuild {
			uncached = append(uncached, up)
		}
	}
	return uncached
}

// AlwaysRunsWarning explains why the cached task name can never hit, or
// returns "" when it can.
func AlwaysRunsWarning(cfg *Config, name string) string {
	if cfg.Tasks[name].Cache == nil {
		return ""
	}
	uncached := UncachedUpstreams(cfg, name)
	switch len(uncached) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%s has cache enabled but always runs: it depends on %s, which is not cached", name, uncached[0])
	}
	return fmt.Sprintf("%s has cache enabled but always runs: it depends on %s, which are not cached", name, strings.Join(uncached, ", "))
}
