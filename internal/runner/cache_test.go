package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/cache"
	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// newCacheHarness returns a harness whose runs share one store, with
// src/a.txt as the only input file.
func newCacheHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.opts.Cache = cache.Open(t.TempDir(), "test")
	h.write("src/a.txt", "a")
	return h
}

func (h *harness) write(rel, content string) {
	h.t.Helper()
	path := filepath.Join(h.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// generate concatenates src/*.txt into gen/<name>.txt and logs its name.
func generate(name string, depends ...string) config.Task {
	task := sh("mkdir -p gen && cat src/*.txt > gen/"+name+".txt && echo "+name+" >> ran.log", depends...)
	task.Cache = &config.TaskCache{Inputs: []string{"src/*.txt"}, Outputs: []string{"gen/" + name + ".txt"}}
	return task
}

func assertRan(t *testing.T, h *harness, want ...string) {
	t.Helper()
	if ran := h.ran(); !slices.Equal(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
}

func TestCache_SecondRunHits(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"gen": generate("gen")}

	first := h.run(context.Background(), tasks, "gen")["gen"]
	if first.Cached || first.Cache == nil || !first.Cache.Stored || first.Cache.Miss == nil || !first.Cache.Miss.NoPrevious {
		t.Fatalf("first run: %+v %+v", first, first.Cache)
	}
	second := h.run(context.Background(), tasks, "gen")["gen"]
	if second.State != runner.Succeeded || !second.Cached || second.Saved <= 0 {
		t.Fatalf("second run should hit with the saved time: %+v", second)
	}
	if !slices.Equal(h.rec.events, []string{"finish gen"}) {
		t.Errorf("a hit is reported finished without starting, got %v", h.rec.events)
	}
	assertRan(t, h, "gen")
}

func TestCache_InputEditMisses(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"gen": generate("gen")}
	h.run(context.Background(), tasks, "gen")
	h.write("src/a.txt", "changed")
	r := h.run(context.Background(), tasks, "gen")["gen"]
	if r.Cached {
		t.Fatal("edited input should miss")
	}
	if got := r.Cache.Miss.Inputs.Modified; !slices.Equal(got, []string{"src/a.txt"}) {
		t.Errorf("miss names modified %v", got)
	}
	assertRan(t, h, "gen", "gen")
}

func TestCache_BranchSwitchHitsEarlierEntry(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"gen": generate("gen")}
	h.run(context.Background(), tasks, "gen")
	h.write("src/a.txt", "b")
	h.run(context.Background(), tasks, "gen")
	// Switching back restores both the input and the committed output.
	h.write("src/a.txt", "a")
	h.write("gen/gen.txt", "a")
	if r := h.run(context.Background(), tasks, "gen")["gen"]; !r.Cached {
		t.Errorf("returning to an earlier state should hit: %+v", r.Cache)
	}
	assertRan(t, h, "gen", "gen")
}

func TestCache_ChangedOrDeletedOutputReruns(t *testing.T) {
	for name, change := range map[string]func(h *harness){
		"edited":  func(h *harness) { h.write("gen/gen.txt", "hand edit") },
		"deleted": func(h *harness) { _ = os.Remove(filepath.Join(h.dir, "gen/gen.txt")) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newCacheHarness(t)
			tasks := map[string]config.Task{"gen": generate("gen")}
			h.run(context.Background(), tasks, "gen")
			change(h)
			r := h.run(context.Background(), tasks, "gen")["gen"]
			if r.Cached || !r.Cache.Miss.OutputsChanged {
				t.Fatalf("changed output should miss: %+v", r.Cache)
			}
			data, _ := os.ReadFile(filepath.Join(h.dir, "gen/gen.txt"))
			if string(data) != "a" {
				t.Errorf("output not regenerated: %q", data)
			}
		})
	}
}

func TestCache_FailureNotStored(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Script += "; exit 1"
	tasks := map[string]config.Task{"gen": task}
	for range 2 {
		if r := h.run(context.Background(), tasks, "gen")["gen"]; r.State != runner.Failed || r.Cached || r.Cache.Stored {
			t.Fatalf("failure: %+v", r)
		}
	}
	assertRan(t, h, "gen", "gen")
}

func TestCache_AllowedFailureNotStoredAndDependentsRunUncached(t *testing.T) {
	h := newCacheHarness(t)
	up := generate("up")
	up.Script += "; exit 1"
	up.ContinueOnError = true
	tasks := map[string]config.Task{"up": up, "down": generate("down", "up")}
	for range 2 {
		results := h.run(context.Background(), tasks, "down")
		if r := results["up"]; r.State != runner.FailedAllowed || r.Cache.Stored {
			t.Fatalf("up: %+v", r)
		}
		if r := results["down"]; r.Cached || r.Cache.Uncached == "" || r.Cache.Stored {
			t.Fatalf("down should run uncached: %+v", r.Cache)
		}
	}
	assertRan(t, h, "up", "down", "up", "down")
}

func TestCache_InterruptedRunNotStored(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Script += "; sleep 30"
	tasks := map[string]config.Task{"gen": task}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	if r := h.run(ctx, tasks, "gen")["gen"]; r.State != runner.Interrupted || r.Cache.Stored {
		t.Fatalf("interrupted: %+v", r)
	}
	if entries := h.opts.Cache.Entries("gen"); len(entries) != 0 {
		t.Errorf("interrupted run stored %d entries", len(entries))
	}
}

func TestCache_HitPublishesKeyToDependents(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"up": generate("up"), "down": generate("down", "up")}
	h.run(context.Background(), tasks, "down")
	results := h.run(context.Background(), tasks, "down")
	if !results["up"].Cached || !results["down"].Cached {
		t.Fatalf("both should hit: up=%+v down=%+v", results["up"].Cache, results["down"].Cache)
	}

	h.write("src/a.txt", "changed")
	results = h.run(context.Background(), tasks, "down")
	if results["up"].Cached || results["down"].Cached {
		t.Fatal("an upstream input change reruns both")
	}
	if got := results["down"].Cache.Miss.Upstream; !slices.Equal(got, []string{"up"}) {
		t.Errorf("down's miss names upstream %v", got)
	}
}

func TestCache_UncachedUpstreamAlwaysRuns(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"tools": logged("tools"), "gen": generate("gen", "tools")}
	for range 2 {
		r := h.run(context.Background(), tasks, "gen")["gen"]
		if r.Cached || !strings.Contains(r.Cache.Uncached, "tools") {
			t.Fatalf("gen should always run: %+v", r.Cache)
		}
	}
	if entries := h.opts.Cache.Entries("gen"); len(entries) != 0 {
		t.Errorf("stored %d entries", len(entries))
	}
}

func TestCache_ForceStores(t *testing.T) {
	h := newCacheHarness(t)
	tasks := map[string]config.Task{"gen": generate("gen")}
	h.run(context.Background(), tasks, "gen")
	h.opts.Force = true
	r := h.run(context.Background(), tasks, "gen")["gen"]
	if r.Cached || !r.Cache.Forced || !r.Cache.Stored {
		t.Fatalf("forced run: %+v %+v", r, r.Cache)
	}
	h.opts.Force = false
	if r := h.run(context.Background(), tasks, "gen")["gen"]; !r.Cached {
		t.Error("run after --force should hit")
	}
	assertRan(t, h, "gen", "gen")
}

func TestCache_DisabledNeitherHashesNorErrors(t *testing.T) {
	h := newHarness(t)
	task := generate("gen")
	task.Cache.Inputs = []string{"missing/*.txt"}
	h.write("src/a.txt", "a")
	tasks := map[string]config.Task{"gen": task}
	for range 2 {
		r := h.run(context.Background(), tasks, "gen")["gen"]
		if r.State != runner.Succeeded || r.Cached || r.Cache != nil {
			t.Fatalf("with the cache disabled: %+v", r)
		}
	}
}

func TestCache_InputPatternMatchingNothingFails(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Cache.Inputs = []string{"missing/*.txt"}
	r := h.run(context.Background(), map[string]config.Task{"gen": task}, "gen")["gen"]
	var noMatch *cache.NoMatchError
	if r.State != runner.Failed || !errors.As(r.Error, &noMatch) {
		t.Fatalf("want a NoMatchError failure, got %+v", r)
	}
	assertRan(t, h)
}

func TestCache_OutputPatternMatchingNothingAfterRunFails(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Cache.Outputs = []string{"gen/typo.txt"}
	r := h.run(context.Background(), map[string]config.Task{"gen": task}, "gen")["gen"]
	var noMatch *cache.NoMatchError
	if r.State != runner.Failed || !errors.As(r.Error, &noMatch) || noMatch.Field != "outputs" {
		t.Fatalf("want an outputs NoMatchError failure, got %+v", r)
	}
}

func TestCache_InputsModifiedDuringRunNotStored(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Script += "; echo more >> src/a.txt"
	r := h.run(context.Background(), map[string]config.Task{"gen": task}, "gen")["gen"]
	if r.State != runner.Succeeded || r.Cache.Stored {
		t.Fatalf("run: %+v %+v", r, r.Cache)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "src/a.txt") {
		t.Errorf("warnings %v should name the changed input", r.Warnings)
	}
}

func TestCache_AfterHookRunsOnHit(t *testing.T) {
	h := newCacheHarness(t)
	hook := logged("hook")
	hook.After = []string{"gen"}
	tasks := map[string]config.Task{"gen": generate("gen"), "hook": hook}
	h.run(context.Background(), tasks, "gen")
	if r := h.run(context.Background(), tasks, "gen"); !r["gen"].Cached || r["hook"].State != runner.Succeeded {
		t.Fatalf("results %+v", r)
	}
	assertRan(t, h, "gen", "hook", "hook")
}

func TestCache_EvictsBeyondConfiguredLimit(t *testing.T) {
	h := newCacheHarness(t)
	task := generate("gen")
	task.Cache.MaxEntries = 2
	tasks := map[string]config.Task{"gen": task}
	for _, content := range []string{"1", "2", "3"} {
		h.write("src/a.txt", content)
		h.run(context.Background(), tasks, "gen")
	}
	if n := len(h.opts.Cache.Entries("gen")); n != 2 {
		t.Errorf("kept %d entries, want 2", n)
	}
}

// buildUpstream returns tasks where gen depends on a build task tool.
func buildUpstream() map[string]config.Task {
	return map[string]config.Task{
		"tool": {Type: config.TaskTypeBuild},
		"gen":  generate("gen", "tool"),
	}
}

func TestCache_BuildUpstreamHitsWhileDerivationUnchanged(t *testing.T) {
	h := newCacheHarness(t)
	h.nix.buildDrvs = map[string]string{"tool": "/nix/store/aaa-tool.drv"}
	tasks := buildUpstream()

	if r := h.run(context.Background(), tasks, "gen")["gen"]; r.Cached || r.Cache.Uncached != "" || !r.Cache.Stored {
		t.Fatalf("first run: %+v %+v", r, r.Cache)
	}
	if r := h.run(context.Background(), tasks, "gen")["gen"]; !r.Cached {
		t.Fatalf("an unchanged derivation upstream should hit: %+v", r.Cache)
	}

	h.nix.buildDrvs["tool"] = "/nix/store/bbb-tool.drv"
	r := h.run(context.Background(), tasks, "gen")["gen"]
	if r.Cached {
		t.Fatal("a changed derivation upstream should miss")
	}
	if got := r.Cache.Miss.Upstream; !slices.Equal(got, []string{"tool (derivation)"}) {
		t.Errorf("miss names upstream %v", got)
	}
	assertRan(t, h, "gen", "gen")
}

func TestCache_ShellUpstreamOfBuildStillAlwaysRuns(t *testing.T) {
	h := newCacheHarness(t)
	h.nix.buildDrvs = map[string]string{"tool": "/nix/store/aaa-tool.drv"}
	tasks := buildUpstream()
	tasks["tool"] = config.Task{Type: config.TaskTypeBuild, Depends: []string{"prep"}}
	tasks["prep"] = logged("prep")
	for range 2 {
		r := h.run(context.Background(), tasks, "gen")["gen"]
		if r.Cached || r.Cache.Uncached != "depends on uncached prep" {
			t.Fatalf("gen should always run: %+v", r.Cache)
		}
	}
}
