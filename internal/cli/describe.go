package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// DescribeCmd shows task details
type DescribeCmd struct {
	Task string `arg:"" help:"Task name to describe"`
}

// Run executes the describe command
func (c *DescribeCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	task, ok := cfg.Tasks[c.Task]
	if !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Print detailed task information
	fmt.Printf("Task: %s\n", c.Task)
	fmt.Println()

	if task.Description != "" {
		fmt.Printf("Description:\n  %s\n\n", task.Description)
	}

	if len(task.Deps) > 0 {
		fmt.Println("Packages:")
		for _, dep := range task.Deps {
			fmt.Printf("  - %s\n", dep)
		}
		fmt.Println()
	}

	if len(task.Depends) > 0 {
		fmt.Println("Depends on:")
		for _, dep := range task.Depends {
			fmt.Printf("  - %s\n", dep)
		}
		fmt.Println()
	}

	// Find tasks that depend on this one
	dependents := findDependents(c.Task, cfg.Tasks)
	if len(dependents) > 0 {
		fmt.Println("Depended on by:")
		for _, dep := range dependents {
			fmt.Printf("  - %s\n", dep)
		}
		fmt.Println()
	}

	if len(task.Inputs) > 0 {
		fmt.Println("Inputs:")
		for _, input := range task.Inputs {
			fmt.Printf("  - %s\n", input)
		}
		fmt.Println()
	}

	if len(task.Outputs) > 0 {
		fmt.Println("Outputs:")
		for _, output := range task.Outputs {
			fmt.Printf("  - %s\n", output)
		}
	}

	return nil
}

func findDependents(taskName string, tasks map[string]config.Task) []string {
	var dependents []string
	target := "task:" + taskName

	for name, task := range tasks {
		for _, dep := range task.Depends {
			if dep == target || dep == taskName {
				dependents = append(dependents, name)
				break
			}
		}
	}

	sort.Strings(dependents)
	return dependents
}
