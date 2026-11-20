package ui

import (
	"fmt"
	"sort"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// Printer prints task and shell listings
type Printer struct{}

// NewPrinter creates a new printer
func NewPrinter() *Printer {
	return &Printer{}
}

// PrintTaskList prints a formatted list of tasks
func (p *Printer) PrintTaskList(tasks map[string]config.Task) {
	if len(tasks) == 0 {
		fmt.Println("No tasks defined")
		return
	}

	// Sort task names for consistent output
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("Tasks:")
	for _, name := range names {
		task := tasks[name]
		if task.Description != "" {
			fmt.Printf("  %-20s %s\n", name, task.Description)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}

// PrintShellList prints a formatted list of dev shells
func (p *Printer) PrintShellList(shells map[string]config.Shell) {
	if len(shells) == 0 {
		return
	}

	// Sort shell names for consistent output
	names := make([]string, 0, len(shells))
	for name := range shells {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("\nDev Shells:")
	for _, name := range names {
		shell := shells[name]
		if shell.Extends != "" {
			fmt.Printf("  %-20s extends %s\n", name, shell.Extends)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}
