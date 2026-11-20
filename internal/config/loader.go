package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// Evaluator evaluates a flake attribute into result.
type Evaluator interface {
	Eval(ctx context.Context, attr string, result any) error
}

// initializeMaps ensures all config maps are initialized to non-nil values
func initializeMaps(cfg *Config) {
	if cfg.Packages == nil {
		cfg.Packages = []string{}
	}
	if cfg.Tasks == nil {
		cfg.Tasks = make(map[string]Task)
	}
	if cfg.DevShells == nil {
		cfg.DevShells = make(map[string]Shell)
	}
}

// Load evaluates nixTasksConfig.<system>, falling back to a system-independent
// nixTasksConfig only when the flake does not provide the per-system attribute.
func Load(ctx context.Context, eval Evaluator) (*Config, error) {
	system := nix.CurrentSystem()
	attr := "nixTasksConfig." + nix.QuoteAttr(system)

	var raw json.RawMessage
	err := eval.Eval(ctx, attr, &raw)
	if err != nil && nix.IsMissingAttribute(err) {
		slog.Debug("no per-system config, falling back", "attr", attr)
		attr = "nixTasksConfig"
		raw = nil
		err = eval.Eval(ctx, attr, &raw)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate %s: %w", attr, err)
	}

	cfg, err := decodeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", attr, err)
	}

	initializeMaps(cfg)
	trimTaskPrefixes(cfg)

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// trimTaskPrefixes strips the optional "task:" prefix from depends and after
// so the rest of nix-tasks only ever sees bare task names.
func trimTaskPrefixes(cfg *Config) {
	trim := func(refs []string) []string {
		out := make([]string, len(refs))
		for i, r := range refs {
			out[i] = strings.TrimPrefix(r, TaskDependencyPrefix)
		}
		return out
	}
	for name, task := range cfg.Tasks {
		task.Depends = trim(task.Depends)
		task.After = trim(task.After)
		cfg.Tasks[name] = task
	}
}

// configJSON is the top level of a config. Each field is nil when its key is
// absent, and stays raw so tasks and shells can be decoded one at a time.
type configJSON struct {
	Packages  *json.RawMessage `json:"packages"`
	Tasks     *json.RawMessage `json:"tasks"`
	DevShells *json.RawMessage `json:"devShells"`
}

// decodeConfig decodes each task and shell separately so a type error names
// the entry it came from; encoding/json omits map keys from its field paths.
func decodeConfig(raw json.RawMessage) (*Config, error) {
	var top configJSON
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("expected an attribute set: %w", err)
	}
	if top.Packages == nil && top.Tasks == nil && top.DevShells == nil {
		return nil, fmt.Errorf("none of packages, tasks, devShells found (got: %s); a per-system config must be exposed as nixTasksConfig.<system>",
			strings.Join(attrNames(raw), ", "))
	}

	cfg := &Config{}
	if top.Packages != nil {
		if err := json.Unmarshal(*top.Packages, &cfg.Packages); err != nil {
			return nil, fmt.Errorf("packages: expected a list of package names (is the flake's nix-tasks input older than this CLI?): %w", err)
		}
	}

	tasks, err := decodeEntries[Task](top.Tasks, "tasks")
	if err != nil {
		return nil, err
	}
	cfg.Tasks = tasks

	shells, err := decodeEntries[Shell](top.DevShells, "devShells")
	if err != nil {
		return nil, err
	}
	cfg.DevShells = shells

	return cfg, nil
}

// attrNames returns the sorted keys of a JSON object, to show what a config
// that has none of the expected keys contains instead.
func attrNames(raw json.RawMessage) []string {
	var attrs map[string]json.RawMessage
	_ = json.Unmarshal(raw, &attrs)
	return sortedKeys(attrs)
}

func decodeEntries[T any](raw *json.RawMessage, section string) (map[string]T, error) {
	if raw == nil {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(*raw, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", section, err)
	}

	out := make(map[string]T, len(entries))
	for _, name := range sortedKeys(entries) {
		var v T
		if err := json.Unmarshal(entries[name], &v); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", section, name, err)
		}
		out[name] = v
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
