package runner_test

import (
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
			name: "empty graph",
			tasks: map[string]config.Task{},
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
				"test":  {Description: "Test", Depends: []string{"task:build"}},
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
				"test": {Description: "Test", Depends: []string{"task:build"}},
			},
			wantErr: true,
			errMsg:  "unknown task",
		},
		{
			name: "simple cycle",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"task:b"}},
				"b": {Depends: []string{"task:a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "self dependency",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"task:a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "longer cycle",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"task:b"}},
				"b": {Depends: []string{"task:c"}},
				"c": {Depends: []string{"task:a"}},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "diamond dependency (no cycle)",
			tasks: map[string]config.Task{
				"a":    {},
				"b":    {Depends: []string{"task:a"}},
				"c":    {Depends: []string{"task:a"}},
				"d":    {Depends: []string{"task:b", "task:c"}},
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
		name       string
		tasks      map[string]config.Task
		target     string
		wantLen    int
		wantFirst  string
		wantLast   string
		wantErr    bool
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
				"test":  {Depends: []string{"task:build"}},
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
				"b": {Depends: []string{"task:a"}},
				"c": {Depends: []string{"task:b"}},
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
				"b":        {Depends: []string{"task:a"}},
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

func TestTaskGraph_ParallelGroups(t *testing.T) {
	tests := []struct {
		name       string
		tasks      map[string]config.Task
		target     string
		wantGroups int
	}{
		{
			name: "single task = one group",
			tasks: map[string]config.Task{
				"build": {},
			},
			target:     "build",
			wantGroups: 1,
		},
		{
			name: "chain = sequential groups",
			tasks: map[string]config.Task{
				"a": {},
				"b": {Depends: []string{"task:a"}},
				"c": {Depends: []string{"task:b"}},
			},
			target:     "c",
			wantGroups: 3,
		},
		{
			name: "parallel tasks = one group",
			tasks: map[string]config.Task{
				"a":   {},
				"b":   {},
				"all": {Depends: []string{"task:a", "task:b"}},
			},
			target:     "all",
			wantGroups: 2,
		},
		{
			name: "diamond = three groups",
			tasks: map[string]config.Task{
				"root":  {},
				"left":  {Depends: []string{"task:root"}},
				"right": {Depends: []string{"task:root"}},
				"top":   {Depends: []string{"task:left", "task:right"}},
			},
			target:     "top",
			wantGroups: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := runner.NewTaskGraph(tt.tasks)
			if err != nil {
				t.Fatalf("NewTaskGraph() error = %v", err)
			}

			groups, err := graph.ParallelGroups(tt.target)
			if err != nil {
				t.Fatalf("ParallelGroups() error = %v", err)
			}

			if len(groups) != tt.wantGroups {
				t.Errorf("ParallelGroups() groups = %d, want %d", len(groups), tt.wantGroups)
				for i, g := range groups {
					t.Logf("  Group %d: %v", i, g)
				}
			}
		})
	}
}

func TestTaskGraph_ParallelGroupsContent(t *testing.T) {
	tasks := map[string]config.Task{
		"base":   {},
		"left":   {Depends: []string{"task:base"}},
		"right":  {Depends: []string{"task:base"}},
		"top":    {Depends: []string{"task:left", "task:right"}},
	}

	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatalf("NewTaskGraph() error = %v", err)
	}

	groups, err := graph.ParallelGroups("top")
	if err != nil {
		t.Fatalf("ParallelGroups() error = %v", err)
	}

	// Verify structure
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	// Group 0: base (no deps)
	if len(groups[0]) != 1 || groups[0][0] != "base" {
		t.Errorf("group 0 should be [base], got %v", groups[0])
	}

	// Group 1: left and right (both depend only on base)
	if len(groups[1]) != 2 {
		t.Errorf("group 1 should have 2 tasks, got %v", groups[1])
	}
	hasLeft := contains(groups[1], "left")
	hasRight := contains(groups[1], "right")
	if !hasLeft || !hasRight {
		t.Errorf("group 1 should contain left and right, got %v", groups[1])
	}

	// Group 2: top (depends on left and right)
	if len(groups[2]) != 1 || groups[2][0] != "top" {
		t.Errorf("group 2 should be [top], got %v", groups[2])
	}
}

func TestTaskGraph_Dependencies(t *testing.T) {
	tasks := map[string]config.Task{
		"a": {},
		"b": {Depends: []string{"task:a"}},
		"c": {Depends: []string{"task:a", "task:b"}},
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

	// Test Dependents (reverse)
	aDependents := graph.Dependents("a")
	if len(aDependents) != 2 {
		t.Errorf("Dependents(a) = %v, want [b, c]", aDependents)
	}

	bDependents := graph.Dependents("b")
	if len(bDependents) != 1 || bDependents[0] != "c" {
		t.Errorf("Dependents(b) = %v, want [c]", bDependents)
	}

	cDependents := graph.Dependents("c")
	if len(cDependents) != 0 {
		t.Errorf("Dependents(c) = %v, want []", cDependents)
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
