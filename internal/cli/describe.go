package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// DescribeCmd shows task details
type DescribeCmd struct {
	Task string `arg:"" completion-predictor:"task" help:"Task name to describe"`
}

// Run executes the describe command
func (c *DescribeCmd) Run(globals *Globals) error {
	ctx := context.Background()

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	if _, ok := cfg.Tasks[c.Task]; !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return fmt.Errorf("failed to build task graph: %w", err)
	}

	describeTask(os.Stdout, graph, c.Task)
	return nil
}

// describeTask writes the details of the named task, which must be in graph.
func describeTask(w io.Writer, graph *runner.TaskGraph, name string) {
	task, _ := graph.Task(name)

	fmt.Fprintf(w, "%s %s\n\n", ui.Label("Task:"), name)

	if task.Description != "" {
		fmt.Fprintf(w, "%s\n  %s\n\n", ui.Label("Description:"), task.Description)
	}

	if label := describeLabel(name, task, graphLabels(graph)); label != "" {
		fmt.Fprintf(w, "%s\n  %s\n\n", ui.Label("Label:"), label)
	}

	printList(w, "Packages:", task.Deps)
	printList(w, "Depends on:", task.Depends)
	printList(w, "Runs after:", task.After)
	printList(w, "Depended on by:", graph.Dependents(name))
	printList(w, "After-hooked by:", graph.AfterHooks(name))
	printList(w, "Inputs:", task.Inputs)

	if task.Type == config.TaskTypeBuild {
		fmt.Fprintf(w, "%s\n  %s\n", ui.Label("Builds:"), nix.NixTasksBuildsAttr(nix.CurrentSystem(), name))
		fmt.Fprintf(w, "%s\n  %s\n\n", ui.Label("Outputs linked to:"), filepath.Join(".nix-tasks", name)+string(filepath.Separator))
	}

	switch {
	case len(task.Commands) > 0:
		fmt.Fprintf(w, "%s\n", ui.Label("Commands:"))
		for _, cmd := range task.Commands {
			fmt.Fprintf(w, "  %s\n", ui.Muted(cmd))
		}
	case task.Script != "":
		fmt.Fprintf(w, "%s\n", ui.Label("Script:"))
		for _, line := range strings.Split(task.Script, "\n") {
			fmt.Fprintf(w, "  %s\n", ui.Muted(line))
		}
	}
}

// describeLabel says how streamed output names the task, or "" when it uses
// the task's name.
func describeLabel(name string, task config.Task, labels map[string]string) string {
	switch {
	case task.Label != "":
		return task.Label
	case labels[name] != "" && labels[name] != name:
		return labels[name] + " (abbreviated; set `label` to choose one)"
	}
	return ""
}

// graphLabels returns the display labels of every task in graph, which
// depend on the whole config.
func graphLabels(graph *runner.TaskGraph) map[string]string {
	tasks := map[string]config.Task{}
	for _, name := range graph.Tasks() {
		tasks[name], _ = graph.Task(name)
	}
	return config.Labels(tasks)
}

func printList(w io.Writer, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "%s\n", ui.Label(heading))
	for _, item := range items {
		fmt.Fprintf(w, "  %s %s\n", ui.Muted("-"), item)
	}
	fmt.Fprintln(w)
}
