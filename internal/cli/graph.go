package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// GraphCmd shows the execution graph for a task
type GraphCmd struct {
	Task string `arg:"" help:"Task name to show execution graph for"`
}

// Run executes the graph command
func (c *GraphCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, ok := cfg.Tasks[c.Task]; !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}

	// Compute the full execution set so we only show after-hooks that would
	// actually run for this target.
	order, err := graph.ExecutionOrder(c.Task)
	if err != nil {
		return fmt.Errorf("failed to compute execution order: %w", err)
	}
	execSet := make(map[string]bool, len(order))
	for _, name := range order {
		execSet[name] = true
	}

	printTree(graph, c.Task, execSet, nil)

	return nil
}

// graphChild represents a child node in the graph tree.
type graphChild struct {
	name    string
	isAfter bool
}

// printTree prints an indented dependency tree rooted at target. If results
// is non-nil, each node shows a status icon and duration; otherwise nodes show
// their description. After-hooks in execSet are included and annotated.
func printTree(graph *runner.TaskGraph, target string, execSet map[string]bool, results map[string]runner.TaskResult) {
	visited := make(map[string]bool)
	printTreeNode(graph, target, 0, false, "", visited, execSet, results)
}

// printTreeNode recursively prints a single node and its children. parentName
// is the tree parent (used to avoid showing redundant edges for after-hooks).
func printTreeNode(graph *runner.TaskGraph, name string, depth int, isAfter bool, parentName string, visited map[string]bool, execSet map[string]bool, results map[string]runner.TaskResult) {
	duplicate := visited[name]
	visited[name] = true

	indent := strings.Repeat("  ", depth)
	label := formatNodeLabel(graph, name, isAfter, duplicate, results)
	fmt.Println(indent + label)

	// Print failed task output inline when showing results
	if results != nil {
		if r, ok := results[name]; ok && !r.Success && r.Output != "" && !errors.Is(r.Error, context.Canceled) {
			tail, tmpFile := tailAndSave(r.Name, r.Output)
			for _, line := range strings.Split(strings.TrimRight(tail, "\n"), "\n") {
				fmt.Printf("%s    %s\n", indent, line)
			}
			if tmpFile != "" {
				fmt.Printf("%s    %s\n", indent, ui.Gray("full output: "+tmpFile))
			}
		}
	}

	if duplicate {
		return
	}

	children := graphChildren(graph, name, execSet)
	for _, c := range children {
		// After-hooks have an edge back to their target (the hook depends on
		// the target). Skip that edge when the target is already the tree
		// parent — the relationship is already expressed by the tree structure.
		if c.name == parentName {
			continue
		}
		printTreeNode(graph, c.name, depth+1, c.isAfter, name, visited, execSet, results)
	}
}

// formatNodeLabel builds the display label for a tree node.
func formatNodeLabel(graph *runner.TaskGraph, name string, isAfter bool, duplicate bool, results map[string]runner.TaskResult) string {
	if results != nil {
		return formatResultLabel(name, isAfter, duplicate, results)
	}
	return formatGraphLabel(graph, name, isAfter, duplicate)
}

// formatGraphLabel builds a label for the graph command (no results).
func formatGraphLabel(graph *runner.TaskGraph, name string, isAfter bool, duplicate bool) string {
	label := name
	if isAfter {
		label += " " + ui.Yellow("(after)")
	}
	if task, ok := graph.Task(name); ok && task.Description != "" {
		label += " " + ui.Gray(task.Description)
	}
	if duplicate {
		label += " " + ui.Gray("*")
	}
	return label
}

// formatResultLabel builds a label for the run command (with results).
func formatResultLabel(name string, isAfter bool, duplicate bool, results map[string]runner.TaskResult) string {
	r, ok := results[name]

	// Status icon
	status := ui.Green("✓")
	if ok && !r.Success {
		if errors.Is(r.Error, context.Canceled) {
			status = ui.Gray("-")
		} else {
			status = ui.Red("✗")
		}
	}

	label := status + " " + name

	if isAfter {
		label += " " + ui.Yellow("(after)")
	}

	// Duration or cached
	if ok {
		if r.Cached {
			label += " " + ui.Gray("(cached)")
		} else if r.Duration > 0 {
			label += " " + ui.Gray(ui.FormatDuration(r.Duration))
		}
	}

	if duplicate {
		label += " " + ui.Gray("*")
	}

	return label
}

// graphChildren returns the direct dependencies and in-scope after-hooks for a task.
func graphChildren(graph *runner.TaskGraph, name string, execSet map[string]bool) []graphChild {
	deps := graph.Dependencies(name)
	sort.Strings(deps)

	hooks := graph.AfterHooks(name)
	sort.Strings(hooks)

	var children []graphChild
	for _, dep := range deps {
		children = append(children, graphChild{name: dep})
	}
	for _, hook := range hooks {
		if execSet[hook] {
			children = append(children, graphChild{name: hook, isAfter: true})
		}
	}
	return children
}
