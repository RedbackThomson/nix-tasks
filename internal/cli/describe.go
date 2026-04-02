package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
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
	fmt.Printf("%s %s\n", ui.Blue("Task:"), c.Task)
	fmt.Println()

	if task.Description != "" {
		fmt.Printf("%s\n  %s\n\n", ui.Blue("Description:"), task.Description)
	}

	if len(task.Deps) > 0 {
		fmt.Printf("%s\n", ui.Blue("Packages:"))
		for _, dep := range task.Deps {
			fmt.Printf("  %s %s\n", ui.Gray("-"), dep)
		}
		fmt.Println()
	}

	if len(task.Depends) > 0 {
		fmt.Printf("%s\n", ui.Blue("Depends on:"))
		for _, dep := range task.Depends {
			depName := strings.TrimPrefix(dep, "task:")
			fmt.Printf("  %s %s\n", ui.Gray("-"), depName)
		}
		fmt.Println()
	}

	if len(task.After) > 0 {
		fmt.Printf("%s\n", ui.Blue("Runs after:"))
		for _, dep := range task.After {
			depName := strings.TrimPrefix(dep, "task:")
			fmt.Printf("  %s %s\n", ui.Gray("-"), depName)
		}
		fmt.Println()
	}

	// Find tasks that depend on this one
	dependents := findDependents(c.Task, cfg.Tasks)
	if len(dependents) > 0 {
		fmt.Printf("%s\n", ui.Blue("Depended on by:"))
		for _, dep := range dependents {
			fmt.Printf("  %s %s\n", ui.Gray("-"), dep)
		}
		fmt.Println()
	}

	// Find tasks that hook onto this one via "after"
	afterHookers := findAfterHookers(c.Task, cfg.Tasks)
	if len(afterHookers) > 0 {
		fmt.Printf("%s\n", ui.Blue("After-hooked by:"))
		for _, dep := range afterHookers {
			fmt.Printf("  %s %s\n", ui.Gray("-"), dep)
		}
		fmt.Println()
	}

	if len(task.Inputs) > 0 {
		fmt.Printf("%s\n", ui.Blue("Inputs:"))
		for _, input := range task.Inputs {
			fmt.Printf("  %s %s\n", ui.Gray("-"), input)
		}
		fmt.Println()
	}

	if len(task.Outputs) > 0 {
		fmt.Printf("%s\n", ui.Blue("Outputs:"))
		for _, output := range task.Outputs {
			fmt.Printf("  %s %s\n", ui.Gray("-"), output)
		}
	}

	// Show commands if verbose
	if len(task.Commands) > 0 {
		fmt.Printf("%s\n", ui.Blue("Commands:"))
		for _, cmd := range task.Commands {
			fmt.Printf("  %s\n", ui.Gray(cmd))
		}
	} else if task.Script != "" {
		fmt.Printf("%s\n", ui.Blue("Script:"))
		lines := strings.Split(task.Script, "\n")
		for _, line := range lines {
			fmt.Printf("  %s\n", ui.Gray(line))
		}
	}

	return nil
}

func findAfterHookers(taskName string, tasks map[string]config.Task) []string {
	var hookers []string
	target := "task:" + taskName

	for name, task := range tasks {
		for _, dep := range task.After {
			if dep == target || dep == taskName {
				hookers = append(hookers, name)
				break
			}
		}
	}

	sort.Strings(hookers)
	return hookers
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
