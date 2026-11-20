package runner_test

import (
	"context"
	"reflect"
	"testing"

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
