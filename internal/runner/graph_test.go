package runner_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestNewTaskGraph(t *testing.T) {
	tests := []struct {
		name    string
		tasks   map[string]config.Task
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty graph",
			tasks:   map[string]config.Task{},
			wantErr: false,
		},
		{
			name: "single task no deps",
			tasks: map[string]config.Task{
				"build": {Description: "Build"},
			},
			wantErr: false,
		},
		{
			name: "simple dependency",
			tasks: map[string]config.Task{
				"build": {Description: "Build"},
				"test":  {Description: "Test", Depends: []string{"build"}},
			},
			wantErr: false,
		},
		{
			name: "dependency without prefix",
			tasks: map[string]config.Task{
				"build": {Description: "Build"},
				"test":  {Description: "Test", Depends: []string{"build"}},
			},
			wantErr: false,
		},
		{
			name: "unknown dependency",
			tasks: map[string]config.Task{
				"test": {Description: "Test", Depends: []string{"build"}},
			},
			wantErr: true,
			errMsg:  "unknown task",
		},
		{
			name: "simple cycle",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"b"}},
				"b": {Depends: []string{"a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "self dependency",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "longer cycle",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"b"}},
				"b": {Depends: []string{"c"}},
				"c": {Depends: []string{"a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "diamond dependency (no cycle)",
			tasks: map[string]config.Task{
				"a": {},
				"b": {Depends: []string{"a"}},
				"c": {Depends: []string{"a"}},
				"d": {Depends: []string{"b", "c"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.NewTaskGraph(tt.tasks)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTaskGraph() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("NewTaskGraph() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestTaskGraph_ExecutionOrder(t *testing.T) {
	tests := []struct {
		name      string
		tasks     map[string]config.Task
		target    string
		wantLen   int
		wantFirst string
		wantLast  string
		wantErr   bool
	}{
		{
			name: "single task",
			tasks: map[string]config.Task{
				"build": {},
			},
			target:    "build",
			wantLen:   1,
			wantFirst: "build",
			wantLast:  "build",
		},
		{
			name: "task with dependency",
			tasks: map[string]config.Task{
				"build": {},
				"test":  {Depends: []string{"build"}},
			},
			target:    "test",
			wantLen:   2,
			wantFirst: "build",
			wantLast:  "test",
		},
		{
			name: "chain of three",
			tasks: map[string]config.Task{
				"a": {},
				"b": {Depends: []string{"a"}},
				"c": {Depends: []string{"b"}},
			},
			target:    "c",
			wantLen:   3,
			wantFirst: "a",
			wantLast:  "c",
		},
		{
			name: "only includes needed tasks",
			tasks: map[string]config.Task{
				"a":        {},
				"b":        {Depends: []string{"a"}},
				"unneeded": {},
			},
			target:    "b",
			wantLen:   2,
			wantFirst: "a",
			wantLast:  "b",
		},
		{
			name: "unknown target",
			tasks: map[string]config.Task{
				"a": {},
			},
			target:  "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := runner.NewTaskGraph(tt.tasks)
			if err != nil {
				t.Fatalf("NewTaskGraph() error = %v", err)
			}

			order, err := graph.ExecutionOrder(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecutionOrder() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(order) != tt.wantLen {
				t.Errorf("ExecutionOrder() len = %d, want %d", len(order), tt.wantLen)
			}
			if order[0] != tt.wantFirst {
				t.Errorf("ExecutionOrder() first = %s, want %s", order[0], tt.wantFirst)
			}
			if order[len(order)-1] != tt.wantLast {
				t.Errorf("ExecutionOrder() last = %s, want %s", order[len(order)-1], tt.wantLast)
			}
		})
	}
}

func TestTaskGraph_Dependencies(t *testing.T) {
	tasks := map[string]config.Task{
		"a": {},
		"b": {Depends: []string{"a"}},
		"c": {Depends: []string{"a", "b"}},
	}

	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatalf("NewTaskGraph() error = %v", err)
	}

	// Test Dependencies
	aDeps := graph.Dependencies("a")
	if len(aDeps) != 0 {
		t.Errorf("Dependencies(a) = %v, want []", aDeps)
	}

	bDeps := graph.Dependencies("b")
	if len(bDeps) != 1 || bDeps[0] != "a" {
		t.Errorf("Dependencies(b) = %v, want [a]", bDeps)
	}

	cDeps := graph.Dependencies("c")
	if len(cDeps) != 2 {
		t.Errorf("Dependencies(c) = %v, want [a, b]", cDeps)
	}
}

func TestNewTaskGraph_AfterHooks(t *testing.T) {
	tests := []struct {
		name    string
		tasks   map[string]config.Task
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid after-hook",
			tasks: map[string]config.Task{
				"build": {},
				"hook":  {After: []string{"build"}},
			},
			wantErr: false,
		},
		{
			name: "unknown after-hook target",
			tasks: map[string]config.Task{
				"hook": {After: []string{"missing"}},
			},
			wantErr: true,
			errMsg:  "unknown task",
		},
		{
			name: "cycle via after and depends",
			tasks: map[string]config.Task{
				"a": {After: []string{"b"}},
				"b": {Depends: []string{"a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.NewTaskGraph(tt.tasks)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTaskGraph() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("NewTaskGraph() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestTaskGraph_ExecutionOrder_AfterHooks(t *testing.T) {
	tests := []struct {
		name      string
		tasks     map[string]config.Task
		target    string
		wantTasks []string // expected tasks in execution set (order checked for first/last)
		wantFirst string
		wantLast  string
	}{
		{
			name: "basic after-hook pulls in hook task",
			tasks: map[string]config.Task{
				"build": {},
				"hook":  {After: []string{"build"}},
			},
			target:    "build",
			wantTasks: []string{"build", "hook"},
			wantFirst: "build",
			wantLast:  "hook",
		},
		{
			name: "after-hook not triggered when target not run",
			tasks: map[string]config.Task{
				"build":     {},
				"hook":      {After: []string{"build"}},
				"unrelated": {},
			},
			target:    "unrelated",
			wantTasks: []string{"unrelated"},
			wantFirst: "unrelated",
			wantLast:  "unrelated",
		},
		{
			name: "transitive after-hooks",
			tasks: map[string]config.Task{
				"a": {},
				"b": {After: []string{"a"}},
				"c": {After: []string{"b"}},
			},
			target:    "a",
			wantTasks: []string{"a", "b", "c"},
			wantFirst: "a",
			wantLast:  "c",
		},
		{
			name: "after-hook with its own depends",
			tasks: map[string]config.Task{
				"a":    {},
				"d":    {},
				"hook": {After: []string{"a"}, Depends: []string{"d"}},
			},
			target:    "a",
			wantTasks: []string{"a", "d", "hook"},
			wantFirst: "a", // a or d could be first depending on order, but a has no deps
			wantLast:  "hook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := runner.NewTaskGraph(tt.tasks)
			if err != nil {
				t.Fatalf("NewTaskGraph() error = %v", err)
			}

			order, err := graph.ExecutionOrder(tt.target)
			if err != nil {
				t.Fatalf("ExecutionOrder() error = %v", err)
			}

			if len(order) != len(tt.wantTasks) {
				t.Errorf("ExecutionOrder() len = %d, want %d; got %v", len(order), len(tt.wantTasks), order)
			}

			// Check all expected tasks are present
			for _, want := range tt.wantTasks {
				if !contains(order, want) {
					t.Errorf("ExecutionOrder() missing task %q; got %v", want, order)
				}
			}

			// Check last task
			if len(order) > 0 && order[len(order)-1] != tt.wantLast {
				t.Errorf("ExecutionOrder() last = %s, want %s; got %v", order[len(order)-1], tt.wantLast, order)
			}
		})
	}
}

func TestTaskGraph_AfterHooks_Accessor(t *testing.T) {
	tasks := map[string]config.Task{
		"build":  {},
		"hook-a": {After: []string{"build"}},
		"hook-b": {After: []string{"build"}},
	}

	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatalf("NewTaskGraph() error = %v", err)
	}

	hooks := graph.AfterHooks("build")
	if len(hooks) != 2 {
		t.Errorf("AfterHooks(build) = %v, want 2 hooks", hooks)
	}

	if !contains(hooks, "hook-a") || !contains(hooks, "hook-b") {
		t.Errorf("AfterHooks(build) = %v, want [hook-a, hook-b]", hooks)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func TestTaskGraph_Dependents(t *testing.T) {
	graph, err := runner.NewTaskGraph(map[string]config.Task{
		"build": {},
		"test":  {Depends: []string{"build"}},
		"lint":  {Depends: []string{"build"}},
		"hook":  {After: []string{"build"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := graph.Dependents("build"); !slices.Equal(got, []string{"lint", "test"}) {
		t.Errorf("Dependents(build) = %v, want [lint test]", got)
	}
	if got := graph.AfterHooks("build"); !slices.Equal(got, []string{"hook"}) {
		t.Errorf("AfterHooks(build) = %v, want [hook]", got)
	}
	if got := graph.Dependents("test"); len(got) != 0 {
		t.Errorf("Dependents(test) = %v, want none", got)
	}
}

func TestTaskGraph_ExecutionOrder_SeveralTargets(t *testing.T) {
	graph, err := runner.NewTaskGraph(map[string]config.Task{
		"gen":    {},
		"build":  {Depends: []string{"gen"}},
		"lint":   {Depends: []string{"gen"}},
		"docs":   {},
		"notify": {After: []string{"lint"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	order, err := graph.ExecutionOrder("build", "lint")
	if err != nil {
		t.Fatalf("ExecutionOrder() error = %v", err)
	}
	want := []string{"gen", "build", "lint", "notify"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Errorf("ExecutionOrder() = %v, want %v", order, want)
	}

	if _, err := graph.ExecutionOrder("build", "missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected an error naming the missing task, got %v", err)
	}
	if _, err := graph.ExecutionOrder(); err == nil {
		t.Error("expected an error with no targets")
	}
}
