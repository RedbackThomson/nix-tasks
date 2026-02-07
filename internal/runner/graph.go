package runner

import (
	"fmt"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// TaskGraph represents the task dependency DAG
type TaskGraph struct {
	tasks   map[string]config.Task
	edges   map[string][]string // task -> tasks it depends on
	reverse map[string][]string // task -> tasks that depend on it
}

// NewTaskGraph builds a dependency graph from configuration
func NewTaskGraph(tasks map[string]config.Task) (*TaskGraph, error) {
	g := &TaskGraph{
		tasks:   tasks,
		edges:   make(map[string][]string),
		reverse: make(map[string][]string),
	}

	// Initialize edges for all tasks
	for name := range tasks {
		g.edges[name] = []string{}
		g.reverse[name] = []string{}
	}

	// Build edges
	for name, task := range tasks {
		for _, dep := range task.Depends {
			depName := strings.TrimPrefix(dep, config.TaskDependencyPrefix)
			if _, ok := tasks[depName]; !ok {
				return nil, fmt.Errorf("task '%s' depends on unknown task '%s'", name, depName)
			}
			g.edges[name] = append(g.edges[name], depName)
			g.reverse[depName] = append(g.reverse[depName], name)
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

// ExecutionOrder returns tasks in topological order for execution
// The target task and all its dependencies will be included
func (g *TaskGraph) ExecutionOrder(target string) ([]string, error) {
	if _, ok := g.tasks[target]; !ok {
		return nil, fmt.Errorf("task not found: %s", target)
	}

	visited := make(map[string]bool)
	var order []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true

		// Visit dependencies first
		for _, dep := range g.edges[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		order = append(order, name)
		return nil
	}

	if err := visit(target); err != nil {
		return nil, err
	}

	return order, nil
}

// ParallelGroups returns tasks grouped by depth level for parallel execution
// Tasks in the same group have no dependencies on each other and can run in parallel
func (g *TaskGraph) ParallelGroups(target string) ([][]string, error) {
	order, err := g.ExecutionOrder(target)
	if err != nil {
		return nil, err
	}

	// Build a set of tasks we care about
	orderSet := make(map[string]bool)
	for _, name := range order {
		orderSet[name] = true
	}

	// Compute depth for each task
	// Depth is the longest path from any task with no dependencies
	depth := make(map[string]int)
	for _, name := range order {
		maxDepth := 0
		for _, dep := range g.edges[name] {
			// Only consider deps that are in our execution set
			if orderSet[dep] {
				if depth[dep] >= maxDepth {
					maxDepth = depth[dep] + 1
				}
			}
		}
		depth[name] = maxDepth
	}

	// Find max depth
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}

	// Group by depth
	groups := make([][]string, maxDepth+1)
	for i := range groups {
		groups[i] = []string{}
	}
	for _, name := range order {
		d := depth[name]
		groups[d] = append(groups[d], name)
	}

	return groups, nil
}

// Dependencies returns the direct dependencies of a task
func (g *TaskGraph) Dependencies(name string) []string {
	return g.edges[name]
}

// Dependents returns tasks that directly depend on the given task
func (g *TaskGraph) Dependents(name string) []string {
	return g.reverse[name]
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
