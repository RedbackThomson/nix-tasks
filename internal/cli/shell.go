package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// ShellCmd enters a development shell
type ShellCmd struct {
	Name    string `arg:"" optional:"" help:"Shell name (default: default)"`
	Command string `short:"c" help:"Command to run in the shell instead of interactive mode"`
}

// Run executes the shell command
func (c *ShellCmd) Run(globals *Globals) error {
	ctx := context.Background()

	shellName := c.Name
	if shellName == "" {
		shellName = "default"
	}

	eval := nix.NewEvaluator(globals.Flake)
	eval.SetDebug(globals.Debug)

	// Load and validate config
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Verify shell exists
	if _, ok := cfg.DevShells[shellName]; !ok {
		return fmt.Errorf("shell not found: %s\navailable shells: %s", shellName, availableShells(cfg.DevShells))
	}

	// Build the shell attribute
	system := nix.CurrentSystem()
	shellAttr := fmt.Sprintf("devShells.%s.%s", system, shellName)

	if c.Command != "" {
		// Run a single command in the shell
		return runShellCommand(ctx, eval, shellAttr, c.Command, globals.Debug)
	}

	// Launch interactive shell
	return runInteractiveShell(ctx, eval, shellAttr, globals.Debug)
}

// runInteractiveShell launches an interactive nix develop shell
func runInteractiveShell(ctx context.Context, eval *nix.Evaluator, shellAttr string, debug bool) error {
	expr := fmt.Sprintf("%s#%s", eval.FlakePath(), shellAttr)
	args := []string{"develop", expr}

	if debug {
		fmt.Printf("Running: nix %v\n", args)
	}

	cmd := exec.CommandContext(ctx, "nix", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// runShellCommand runs a command inside the shell and exits
func runShellCommand(ctx context.Context, eval *nix.Evaluator, shellAttr, command string, debug bool) error {
	expr := fmt.Sprintf("%s#%s", eval.FlakePath(), shellAttr)
	args := []string{"develop", expr, "--command", "bash", "-c", command}

	if debug {
		fmt.Printf("Running: nix %v\n", args)
	}

	cmd := exec.CommandContext(ctx, "nix", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// availableShells returns a comma-separated list of available shell names
func availableShells(shells map[string]config.Shell) string {
	if len(shells) == 0 {
		return "(none)"
	}

	names := make([]string, 0, len(shells))
	for name := range shells {
		names = append(names, name)
	}

	result := ""
	for i, name := range names {
		if i > 0 {
			result += ", "
		}
		result += name
	}
	return result
}
