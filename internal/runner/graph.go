package runner

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// TaskGraph represents the task dependency DAG
type TaskGraph struct {
	tasks      map[string]config.Task
	edges      map[string][]string // task -> tasks it depends on
	dependents map[string][]string // task -> tasks that list it in "depends"
	afterHooks map[string][]string // target task -> tasks that run after it via "after"
}

// NewTaskGraph builds a dependency graph from configuration
func NewTaskGraph(tasks map[string]config.Task) (*TaskGraph, error) {
	g := &TaskGraph{
		tasks:      tasks,
		edges:      make(map[string][]string),
		dependents: make(map[string][]string),
		afterHooks: make(map[string][]string),
	}

	// Initialize edges for all tasks
	for name := range tasks {
		g.edges[name] = []string{}
		g.afterHooks[name] = []string{}
	}

	// Build edges from depends
	for name, task := range tasks {
		for _, depName := range task.Depends {
			if _, ok := tasks[depName]; !ok {
				return nil, fmt.Errorf("task '%s' depends on unknown task '%s'", name, depName)
			}
			g.edges[name] = append(g.edges[name], depName)
			g.dependents[depName] = append(g.dependents[depName], name)
		}
	}

	// Build edges from after (hook task depends on its target)
	for name, task := range tasks {
		for _, depName := range task.After {
			if _, ok := tasks[depName]; !ok {
				return nil, fmt.Errorf("task '%s' has after-hook on unknown task '%s'", name, depName)
			}
			g.afterHooks[depName] = append(g.afterHooks[depName], name)
			g.edges[name] = append(g.edges[name], depName)
		}
	}

	// Check for cycles
	if cycle := g.findCycle(); cycle != nil {
		return nil, fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " -> "))
	}

	return g, nil
}

// findCycle returns a cycle if one exists, nil otherwise
func (g *TaskGraph) findCycle() []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string, path []string) []string
	dfs = func(node string, path []string) []string {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, dep := range g.edges[node] {
			if !visited[dep] {
				if cycle := dfs(dep, path); cycle != nil {
					return cycle
				}
			} else if recStack[dep] {
				// Found cycle - return path from dep to current node, then back to dep
				return append(path, dep)
			}
		}

		recStack[node] = false
		return nil
	}

	for name := range g.tasks {
		if !visited[name] {
			if cycle := dfs(name, nil); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// ExecutionOrder returns tasks in topological order for execution.
// The target task, all its dependencies, and any after-hooks (transitively)
// will be included.
func (g *TaskGraph) ExecutionOrder(target string) ([]string, error) {
	if _, ok := g.tasks[target]; !ok {
		return nil, fmt.Errorf("task not found: %s", target)
	}

	inSet := make(map[string]bool)

	var collectDeps func(name string)
	collectDeps = func(name string) {
		if inSet[name] {
			return
		}
		inSet[name] = true
		for _, dep := range g.edges[name] {
			collectDeps(dep)
		}
	}

	// Start with target and its transitive dependencies
	collectDeps(target)

	// Expand with after-hooks until stable
	changed := true
	for changed {
		changed = false
		for _, task := range sortedKeys(inSet) {
			for _, hook := range g.afterHooks[task] {
				if !inSet[hook] {
					collectDeps(hook)
					changed = true
				}
			}
		}
	}

	visited := make(map[string]bool)
	var order []string

	var visit func(name string)
	visit = func(name string) {
		if visited[name] || !inSet[name] {
			return
		}
		visited[name] = true
		for _, dep := range g.edges[name] {
			visit(dep)
		}
		order = append(order, name)
	}

	// Visit from target first, then any remaining tasks in the set
	// (after-hooks may not be reachable from target via edges alone)
	visit(target)
	for _, task := range sortedKeys(inSet) {
		visit(task)
	}

	return order, nil
}

// Dependencies returns the direct dependencies of a task
func (g *TaskGraph) Dependencies(name string) []string {
	return g.edges[name]
}

// AfterHooks returns the sorted tasks that run after the given task via the
// "after" field.
func (g *TaskGraph) AfterHooks(name string) []string {
	return slices.Sorted(slices.Values(g.afterHooks[name]))
}

// Dependents returns the sorted tasks that list the given task in "depends".
func (g *TaskGraph) Dependents(name string) []string {
	return slices.Sorted(slices.Values(g.dependents[name]))
}

// Tasks returns all task names in the graph
func (g *TaskGraph) Tasks() []string {
	names := make([]string, 0, len(g.tasks))
	for name := range g.tasks {
		names = append(names, name)
	}
	return names
}

// Task returns a task by name
func (g *TaskGraph) Task(name string) (config.Task, bool) {
	task, ok := g.tasks[name]
	return task, ok
}

func sortedKeys(m map[string]bool) []string {
	return slices.Sorted(maps.Keys(m))
}
