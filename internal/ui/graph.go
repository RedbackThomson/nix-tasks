package ui

import (
	"sort"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// GraphTree renders the tasks that running target would execute as an
// indented tree, one line per node, with each task's description. After-hooks
// that would run are included and annotated; a task already shown is marked
// with * and not expanded again.
func GraphTree(graph *runner.TaskGraph, target string) ([]string, error) {
	order, err := graph.ExecutionOrder(target)
	if err != nil {
		return nil, err
	}
	execSet := make(map[string]bool, len(order))
	for _, name := range order {
		execSet[name] = true
	}

	t := treeRenderer{graph: graph, execSet: execSet, visited: map[string]bool{}}
	t.node(target, 0, false, "")
	return t.lines, nil
}

type treeRenderer struct {
	graph   *runner.TaskGraph
	execSet map[string]bool
	visited map[string]bool
	lines   []string
}

// node renders name and its children. parentName is the tree parent, used to
// avoid showing redundant edges for after-hooks.
func (t *treeRenderer) node(name string, depth int, isAfter bool, parentName string) {
	duplicate := t.visited[name]
	t.visited[name] = true

	t.lines = append(t.lines, strings.Repeat("  ", depth)+t.label(name, isAfter, duplicate))
	if duplicate {
		return
	}

	for _, c := range t.children(name) {
		// After-hooks have an edge back to their target (the hook depends on
		// the target). Skip that edge when the target is already the tree
		// parent — the relationship is already expressed by the tree structure.
		if c.name == parentName {
			continue
		}
		t.node(c.name, depth+1, c.isAfter, name)
	}
}

func (t *treeRenderer) label(name string, isAfter, duplicate bool) string {
	label := name
	if isAfter {
		label += " " + Label("(after)")
	}
	task, _ := t.graph.Task(name)
	if task.Cache != nil {
		label += " " + Muted("(cache)")
	}
	if task.Description != "" {
		label += " " + Muted(task.Description)
	}
	if duplicate {
		label += " " + Muted("*")
	}
	return label
}

type graphChild struct {
	name    string
	isAfter bool
}

// children returns the direct dependencies and in-scope after-hooks of a task.
func (t *treeRenderer) children(name string) []graphChild {
	deps := t.graph.Dependencies(name)
	sort.Strings(deps)

	hooks := t.graph.AfterHooks(name)

	var children []graphChild
	for _, dep := range deps {
		children = append(children, graphChild{name: dep})
	}
	for _, hook := range hooks {
		if t.execSet[hook] {
			children = append(children, graphChild{name: hook, isAfter: true})
		}
	}
	return children
}
