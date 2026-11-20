package cli

import (
	"context"
	"fmt"
	"path/filepath"
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

	cfg, _, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	task, ok := cfg.Tasks[c.Task]
	if !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Print detailed task information
	fmt.Printf("%s %s\n", ui.Label("Task:"), c.Task)
	fmt.Println()

	if task.Description != "" {
		fmt.Printf("%s\n  %s\n\n", ui.Label("Description:"), task.Description)
	}

	if len(task.Deps) > 0 {
		fmt.Printf("%s\n", ui.Label("Packages:"))
		for _, dep := range task.Deps {
			fmt.Printf("  %s %s\n", ui.Muted("-"), dep)
		}
		fmt.Println()
	}

	if len(task.Depends) > 0 {
		fmt.Printf("%s\n", ui.Label("Depends on:"))
		for _, dep := range task.Depends {
			fmt.Printf("  %s %s\n", ui.Muted("-"), dep)
		}
		fmt.Println()
	}

	if len(task.After) > 0 {
		fmt.Printf("%s\n", ui.Label("Runs after:"))
		for _, dep := range task.After {
			fmt.Printf("  %s %s\n", ui.Muted("-"), dep)
		}
		fmt.Println()
	}

	// Find tasks that depend on this one
	dependents := findDependents(c.Task, cfg.Tasks)
	if len(dependents) > 0 {
		fmt.Printf("%s\n", ui.Label("Depended on by:"))
		for _, dep := range dependents {
			fmt.Printf("  %s %s\n", ui.Muted("-"), dep)
		}
		fmt.Println()
	}

	// Find tasks that hook onto this one via "after"
	afterHookers := findAfterHookers(c.Task, cfg.Tasks)
	if len(afterHookers) > 0 {
		fmt.Printf("%s\n", ui.Label("After-hooked by:"))
		for _, dep := range afterHookers {
			fmt.Printf("  %s %s\n", ui.Muted("-"), dep)
		}
		fmt.Println()
	}

	if len(task.Inputs) > 0 {
		fmt.Printf("%s\n", ui.Label("Inputs:"))
		for _, input := range task.Inputs {
			fmt.Printf("  %s %s\n", ui.Muted("-"), input)
		}
		fmt.Println()
	}

	if task.Type == config.TaskTypeBuild {
		fmt.Printf("%s\n  %s\n", ui.Label("Builds:"), nix.NixTasksBuildsAttr(nix.CurrentSystem(), c.Task))
		fmt.Printf("%s\n  %s\n\n", ui.Label("Outputs linked to:"), filepath.Join(".nix-tasks", c.Task)+string(filepath.Separator))
	}

	// Show commands if verbose
	if len(task.Commands) > 0 {
		fmt.Printf("%s\n", ui.Label("Commands:"))
		for _, cmd := range task.Commands {
			fmt.Printf("  %s\n", ui.Muted(cmd))
		}
	} else if task.Script != "" {
		fmt.Printf("%s\n", ui.Label("Script:"))
		lines := strings.Split(task.Script, "\n")
		for _, line := range lines {
			fmt.Printf("  %s\n", ui.Muted(line))
		}
	}

	return nil
}

func findAfterHookers(taskName string, tasks map[string]config.Task) []string {
	var hookers []string

	for name, task := range tasks {
		for _, dep := range task.After {
			if dep == taskName {
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

	for name, task := range tasks {
		for _, dep := range task.Depends {
			if dep == taskName {
				dependents = append(dependents, name)
				break
			}
		}
	}

	sort.Strings(dependents)
	return dependents
}
