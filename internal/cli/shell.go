package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

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

	cfg, eval, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	// Verify shell exists
	if _, ok := cfg.DevShells[shellName]; !ok {
		return fmt.Errorf("shell not found: %s\navailable shells: %s", shellName, availableShells(cfg.DevShells))
	}

	shellAttr := nix.DevShellAttr(nix.CurrentSystem(), shellName)
	return execNix(developArgs(eval.FlakePath(), shellAttr, c.Command))
}

// developArgs returns the nix argv (including argv[0]) for entering a shell,
// or for running command in it when command is non-empty.
func developArgs(flakePath, shellAttr, command string) []string {
	args := []string{"nix", "develop", fmt.Sprintf("%s#%s", flakePath, shellAttr)}
	if command != "" {
		args = append(args, "--command", "bash", "-c", command)
	}
	return args
}

// execNix replaces this process with nix so exit codes, signals and job
// control behave exactly as if the user had run nix develop directly.
func execNix(args []string) error {
	path, err := exec.LookPath("nix")
	if err != nil {
		return fmt.Errorf("nix not found: %w", err)
	}

	slog.Debug("exec", "path", path, "args", args)
	_ = os.Stdout.Sync()
	_ = os.Stderr.Sync()

	if err := syscall.Exec(path, args, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec %s: %w", path, err)
	}
	return nil
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
