package runner_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

type planRecorder struct {
	*recorder
	plans []runner.RunPlan
}

func (p *planRecorder) RunPlanned(plan runner.RunPlan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plans = append(p.plans, plan)
	p.events = append(p.events, "plan")
}

func TestRun_ReportsPlanFirst(t *testing.T) {
	tasks := map[string]config.Task{
		"deploy":  sh("true", "test", "build"),
		"test":    sh("true", "gen"),
		"build":   sh("true", "gen"),
		"gen":     sh("true"),
		"lint":    sh("true"),
		"unused":  sh("true"),
		"release": {Script: "true", Depends: []string{"deploy", "lint"}, Label: "rel"},
	}
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	rec := &planRecorder{recorder: newRecorder()}
	ex := runner.NewExecutor(&fakeNix{}, &config.Config{Tasks: tasks}, runner.Options{
		Jobs: 3, FlakeRoot: t.TempDir(), DevEnvDir: t.TempDir(), Reporter: rec,
	})
	if _, err := ex.Run(context.Background(), graph, "release"); err != nil {
		t.Fatal(err)
	}

	if len(rec.plans) != 1 || rec.events[0] != "plan" {
		t.Fatalf("expected one plan before any other event, got events %v", rec.events)
	}
	plan := rec.plans[0]
	if want := []string{"gen", "build", "lint", "test", "deploy", "release"}; !reflect.DeepEqual(plan.Tasks, want) {
		t.Errorf("Tasks = %v, want %v", plan.Tasks, want)
	}
	if !reflect.DeepEqual(plan.Targets, []string{"release"}) || plan.Jobs != 3 {
		t.Errorf("Targets, Jobs = %q, %d", plan.Targets, plan.Jobs)
	}
	if plan.Labels["release"] != "rel" || plan.Labels["unused"] != "unused" {
		t.Errorf("Labels = %v", plan.Labels)
	}
}

func TestRun_PlanCoversEveryTarget(t *testing.T) {
	tasks := map[string]config.Task{
		"gen":   sh("true"),
		"build": sh("true", "gen"),
		"lint":  sh("true"),
	}
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	rec := &planRecorder{recorder: newRecorder()}
	ex := runner.NewExecutor(&fakeNix{}, &config.Config{Tasks: tasks}, runner.Options{
		FlakeRoot: t.TempDir(), DevEnvDir: t.TempDir(), Reporter: rec,
	})
	if _, err := ex.Run(context.Background(), graph, "lint", "build"); err != nil {
		t.Fatal(err)
	}
	plan := rec.plans[0]
	if !reflect.DeepEqual(plan.Targets, []string{"lint", "build"}) {
		t.Errorf("Targets = %v", plan.Targets)
	}
	if want := []string{"gen", "build", "lint"}; !reflect.DeepEqual(plan.Tasks, want) {
		t.Errorf("Tasks = %v, want %v", plan.Tasks, want)
	}
}

type setupRecorder struct {
	*recorder
	envs [][]runner.EnvSetup
}

func (s *setupRecorder) SetupFinished(envs []runner.EnvSetup, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envs = append(s.envs, envs)
	s.events = append(s.events, "setup")
}

func TestRun_PreparesEnvironmentsUpFront(t *testing.T) {
	tasks := map[string]config.Task{
		"a":      sh("true"),
		"b":      sh("true"),
		"c":      sh("true"),
		"broken": sh("true"),
		"all":    {Depends: []string{"a", "b", "c", "broken"}},
	}
	fn := &fakeNix{
		drvs:   map[string]string{"c": "/nix/store/other-nix-shell.drv"},
		broken: map[string]bool{"broken": true},
	}
	devEnvDir := t.TempDir()
	run := func() (*setupRecorder, map[string]runner.TaskResult) {
		graph, err := runner.NewTaskGraph(tasks)
		if err != nil {
			t.Fatal(err)
		}
		rec := &setupRecorder{recorder: newRecorder()}
		ex := runner.NewExecutor(fn, &config.Config{Tasks: tasks}, runner.Options{
			FlakeRoot: t.TempDir(), DevEnvDir: devEnvDir, Reporter: rec, ContinueOnError: true,
		})
		results, err := ex.Run(context.Background(), graph, "all")
		if err != nil {
			t.Fatal(err)
		}
		byName := map[string]runner.TaskResult{}
		for _, r := range results {
			byName[r.Name] = r
		}
		return rec, byName
	}

	rec, results := run()
	if len(rec.envs) != 1 || rec.events[0] != "setup" {
		t.Fatalf("expected setup before any task event, got %v", rec.events)
	}
	envs := rec.envs[0]
	var got []string
	for _, e := range envs {
		got = append(got, fmt.Sprintf("%v cached=%v err=%v", e.Tasks, e.Cached, e.Err != nil))
	}
	want := []string{
		"[a b] cached=false err=false",
		"[broken] cached=false err=true",
		"[c] cached=false err=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("envs =\n%v\nwant\n%v", got, want)
	}
	if results["broken"].State != runner.Failed || results["a"].State != runner.Succeeded || results["c"].State != runner.Succeeded {
		t.Errorf("unexpected states: %v", results)
	}
	if len(fn.realised) != 2 {
		t.Errorf("realised %v, want each shell once", fn.realised)
	}

	rec, _ = run()
	for _, e := range rec.envs[0] {
		if e.Err == nil && !e.Cached {
			t.Errorf("%v: expected the second run to use the cached environment", e.Tasks)
		}
	}
}

func TestRun_SkipReasons(t *testing.T) {
	h := newHarness(t)
	h.opts.ContinueOnError = true
	results := h.run(context.Background(), map[string]config.Task{
		"lint":   sh("exit 1"),
		"build":  sh("true", "lint"),
		"deploy": sh("true", "build"),
	}, "deploy")
	if got := results["build"].SkipReason; got != "lint failed" {
		t.Errorf("build: SkipReason = %q", got)
	}
	if got := results["deploy"].SkipReason; got != "build skipped" {
		t.Errorf("deploy: SkipReason = %q", got)
	}
}
