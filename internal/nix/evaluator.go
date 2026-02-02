package nix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
)

// CurrentSystem returns the Nix system identifier for the current platform
func CurrentSystem() string {
	arch := runtime.GOARCH
	os := runtime.GOOS

	// Map Go arch to Nix arch
	nixArch := arch
	switch arch {
	case "amd64":
		nixArch = "x86_64"
	case "arm64":
		nixArch = "aarch64"
	}

	// Map Go OS to Nix OS
	nixOS := os
	switch os {
	case "darwin":
		nixOS = "darwin"
	case "linux":
		nixOS = "linux"
	}

	return fmt.Sprintf("%s-%s", nixArch, nixOS)
}

// NixTasksShellsAttr returns the flake attribute for a task shell
func NixTasksShellsAttr(system, taskName string) string {
	return fmt.Sprintf("nixTasksShells.%s.%s", system, taskName)
}

// Evaluator wraps Nix CLI operations
type Evaluator struct {
	flakePath string
	debug     bool
}

// NewEvaluator creates a new Nix evaluator
func NewEvaluator(flakePath string) *Evaluator {
	return &Evaluator{
		flakePath: flakePath,
	}
}

// SetDebug enables debug logging of Nix commands
func (e *Evaluator) SetDebug(debug bool) {
	e.debug = debug
}

// FlakePath returns the configured flake path
func (e *Evaluator) FlakePath() string {
	return e.flakePath
}

// Eval evaluates a flake attribute and unmarshals the JSON result
func (e *Evaluator) Eval(ctx context.Context, attr string, result any) error {
	expr := fmt.Sprintf("%s#%s", e.flakePath, attr)
	args := []string{"eval", "--json", expr}

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	cmd := exec.CommandContext(ctx, "nix", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &EvalError{
			Attribute: attr,
			Stderr:    stderr.String(),
			Err:       err,
		}
	}

	if err := json.Unmarshal(stdout.Bytes(), result); err != nil {
		return fmt.Errorf("failed to parse nix output: %w", err)
	}

	return nil
}

// Build builds a flake attribute and returns the store path
func (e *Evaluator) Build(ctx context.Context, attr string) (string, error) {
	expr := fmt.Sprintf("%s#%s", e.flakePath, attr)
	args := []string{"build", "--no-link", "--print-out-paths", expr}

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	cmd := exec.CommandContext(ctx, "nix", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &BuildError{
			Attribute: attr,
			Stderr:    stderr.String(),
			Err:       err,
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// DevelopCmd returns an exec.Cmd that runs a command inside nix develop
func (e *Evaluator) DevelopCmd(ctx context.Context, shellAttr string, command []string) *exec.Cmd {
	expr := fmt.Sprintf("%s#%s", e.flakePath, shellAttr)
	args := []string{"develop", expr, "--command"}
	args = append(args, command...)

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	return exec.CommandContext(ctx, "nix", args...)
}
