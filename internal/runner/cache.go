package runner

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// CacheOutcome is what the cache did for a task that declares one.
type CacheOutcome struct {
	// Uncached says why the task ran without consulting the cache; empty
	// when the cache was consulted.
	Uncached string
	// Forced is set when --force skipped the lookup.
	Forced bool
	// Miss explains a lookup that found no usable entry.
	Miss *cache.Explanation
	// Stored is set when the run's result was stored.
	Stored bool
}

// cachePlan is decided when a task is started, from its dependencies'
// results.
type cachePlan struct {
	// uncached is why the task cannot use the cache, or "".
	uncached string
	upstream map[string]string
	// upstreamDrvs holds the drvPath of each direct build-task dependency.
	upstreamDrvs map[string]string
	// buildKey is published by a build task that succeeds, so cached tasks
	// downstream of it can hit.
	buildKey string
}

// planCache decides whether name may use the cache. Every shell task
// upstream must itself be cached and have succeeded: an uncached one can
// change anything without the cache seeing it, and a failed one may have
// left partial products. A build task upstream stands in with its
// derivation path, which changes whenever what it builds does.
func (e *Executor) planCache(graph *TaskGraph, name string, results map[string]TaskResult, keys map[string]string) cachePlan {
	task := graph.tasks[name]
	if e.options.Cache == nil {
		return cachePlan{}
	}
	drv := ""
	switch {
	case task.Type == config.TaskTypeBuild:
		if drv = e.buildDrvs[name]; drv == "" {
			return cachePlan{}
		}
	case task.Cache == nil:
		return cachePlan{}
	default:
		cfg := &config.Config{Tasks: graph.tasks}
		if uncached := config.UncachedUpstreams(cfg, name); len(uncached) > 0 {
			return cachePlan{uncached: "depends on uncached " + strings.Join(uncached, ", ")}
		}
	}

	deps := slices.Clone(graph.Dependencies(name))
	slices.Sort(deps)
	plan := cachePlan{upstream: map[string]string{}, upstreamDrvs: map[string]string{}}
	for _, dep := range slices.Compact(deps) {
		if r := results[dep]; r.State == FailedAllowed {
			return cachePlan{uncached: "upstream " + dep + " failed"}
		}
		key, ok := keys[dep]
		if !ok {
			if graph.tasks[dep].Type == config.TaskTypeBuild && e.buildDrvsErr != nil {
				return cachePlan{uncached: "could not evaluate the derivation of " + dep}
			}
			return cachePlan{uncached: "upstream " + dep + " ran without the cache"}
		}
		plan.upstream[dep] = key
		if d := e.buildDrvs[dep]; d != "" {
			plan.upstreamDrvs[dep] = d
		}
	}
	if drv != "" {
		plan.buildKey = cache.BuildKey(drv, plan.upstream)
	}
	return plan
}

// evalBuildDrvs evaluates the derivation path of every build task upstream
// of a cached task in the run, in one evaluation and without building.
func (e *Executor) evalBuildDrvs(ctx context.Context, graph *TaskGraph, order []string) {
	if e.options.Cache == nil {
		return
	}
	cfg := &config.Config{Tasks: graph.tasks}
	needed := map[string]bool{}
	for _, name := range order {
		if graph.tasks[name].Cache == nil {
			continue
		}
		for _, up := range config.Upstream(cfg, name) {
			if graph.tasks[up].Type == config.TaskTypeBuild {
				needed[up] = true
			}
		}
	}
	if len(needed) == 0 {
		return
	}
	names := slices.Sorted(maps.Keys(needed))
	start := time.Now()
	e.buildDrvs, e.buildDrvsErr = e.nix.BuildDrvPaths(ctx, nix.CurrentSystem(), names)
	slog.Debug("evaluated build derivations for the cache", "tasks", names, "duration", time.Since(start), "error", e.buildDrvsErr)
}

// runTask runs a task, answering from the cache when the task declares one,
// its key matches a stored entry and the outputs in the working tree are the
// ones that entry recorded. Only complete, successful runs are stored, so a
// failure is always retried.
func (e *Executor) runTask(ctx context.Context, name string, task config.Task, plan cachePlan, r *TaskResult) error {
	exec := func() error {
		log, err := e.execTask(ctx, name, task)
		r.Log = log
		return err
	}
	store := e.options.Cache
	if task.Cache == nil || store == nil {
		err := exec()
		if err == nil {
			r.key = plan.buildKey
		}
		return err
	}
	r.Cache = &CacheOutcome{Uncached: plan.uncached}
	if plan.uncached != "" {
		slog.Debug("running without the cache", "task", name, "reason", plan.uncached)
		return exec()
	}

	hashStart := time.Now()
	key, err := cache.ComputeKey(cache.Request{
		Root:         e.options.FlakeRoot,
		System:       nix.CurrentSystem(),
		Task:         task,
		Upstream:     plan.upstream,
		UpstreamDrvs: plan.upstreamDrvs,
	})
	if err != nil {
		return err
	}
	slog.Debug("computed cache key", "task", name, "key", key.Hash, "inputs", len(key.Detail.Inputs), "duration", time.Since(hashStart))

	if e.options.Force {
		r.Cache.Forced = true
	} else {
		hit, miss, err := e.lookup(name, task, key)
		if err != nil {
			return err
		}
		if hit != nil {
			if err := store.Touch(hit); err != nil {
				r.Warnings = append(r.Warnings, "could not update cache entry: "+err.Error())
			}
			r.Cached = true
			r.Saved = time.Duration(hit.RunDuration)
			r.key = key.Hash
			return nil
		}
		r.Cache.Miss = miss
		slog.Debug("cache miss", "task", name, "reason", miss.Reason())
	}

	runStart := time.Now()
	if err := exec(); err != nil {
		return err
	}
	runDuration := time.Since(runStart)
	if ctx.Err() != nil {
		return nil
	}

	outputs, err := cache.HashOutputs(e.options.FlakeRoot, task.Cache, false)
	if err != nil {
		return err
	}
	after, err := cache.HashInputs(e.options.FlakeRoot, task.Cache)
	if err != nil {
		r.Warnings = append(r.Warnings, "not cached: "+err.Error())
		return nil
	}
	// A task that writes to its own inputs would otherwise miss forever, and
	// one whose inputs were edited mid-run may have seen a mix.
	if changes := cache.DiffFiles(key.Detail.Inputs, after); !changes.Empty() {
		r.Warnings = append(r.Warnings, fmt.Sprintf("not cached: inputs changed while it ran: %s", strings.Join(slices.Concat(changes.Modified, changes.Added, changes.Removed), ", ")))
		return nil
	}
	r.key = key.Hash

	detail := key.Detail
	detail.Outputs = outputs
	entry := &cache.Entry{
		Task:         name,
		Key:          key.Hash,
		RunDuration:  cache.Duration(runDuration),
		Components:   key.Components,
		Detail:       detail,
		OutputDigest: cache.OutputDigest(outputs),
	}
	if err := store.Put(entry, cache.MaxEntries(task.Cache.MaxEntries, e.config.CacheSettings.MaxEntriesPerTask)); err != nil {
		r.Warnings = append(r.Warnings, "could not store cache entry: "+err.Error())
		return nil
	}
	r.Cache.Stored = true
	return nil
}

// lookup returns the entry that answers key, or why there is none. An entry
// whose recorded outputs differ from the working tree's does not answer:
// after a branch switch or a hand edit the outputs must be regenerated.
func (e *Executor) lookup(name string, task config.Task, key *cache.Key) (*cache.Entry, *cache.Explanation, error) {
	store := e.options.Cache
	if entry, ok := store.Lookup(name, key.Hash); ok {
		outputs, err := cache.HashOutputs(e.options.FlakeRoot, task.Cache, true)
		if err != nil {
			return nil, nil, err
		}
		if cache.OutputDigest(outputs) == entry.OutputDigest {
			return entry, nil, nil
		}
		x := cache.ExplainOutputs(entry, outputs)
		return nil, &x, nil
	}
	latest, _ := store.Latest(name)
	x := cache.Explain(latest, key)
	return nil, &x, nil
}
