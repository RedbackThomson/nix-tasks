package nix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"time"
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
	return e.eval(ctx, attr, nil, result)
}

func (e *Evaluator) eval(ctx context.Context, attr string, extraArgs []string, result any) error {
	expr := fmt.Sprintf("%s#%s", e.flakePath, attr)
	args := append([]string{"eval", "--json", expr}, extraArgs...)

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	cmd := exec.CommandContext(ctx, "nix", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Run(); err != nil {
		return &EvalError{
			Attribute: attr,
			Stderr:    stderr.String(),
			Err:       err,
		}
	}
	slog.Debug("nix eval complete", "attr", attr, "duration", time.Since(start))

	if err := json.Unmarshal(stdout.Bytes(), result); err != nil {
		return fmt.Errorf("failed to parse nix output: %w", err)
	}

	return nil
}

// BuildCmd returns an exec.Cmd that builds every output of a flake attribute
// and prints them as JSON on stdout (see ParseBuildOutputs). --no-link because
// the caller creates its own links under .nix-tasks/.
func (e *Evaluator) BuildCmd(ctx context.Context, attr string) *exec.Cmd {
	expr := fmt.Sprintf("%s#%s^*", e.flakePath, attr)
	args := []string{"build", expr, "--no-link", "--json"}

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	return exec.CommandContext(ctx, "nix", args...)
}

// ParseBuildOutputs parses `nix build --json` output into a map of output
// name to store path.
func ParseBuildOutputs(data []byte) (map[string]string, error) {
	var results []struct {
		Outputs map[string]string `json:"outputs"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to parse nix build output: %w", err)
	}

	outputs := make(map[string]string)
	for _, r := range results {
		for name, path := range r.Outputs {
			outputs[name] = path
		}
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("nix build reported no outputs")
	}
	return outputs, nil
}

// ShellDrvPaths returns the drvPath of the shell each named task runs in,
// evaluating only those shells.
func (e *Evaluator) ShellDrvPaths(ctx context.Context, system string, tasks []string) (map[string]string, error) {
	wanted := make(map[string]any, len(tasks))
	for _, t := range tasks {
		wanted[t] = nil
	}
	wantedJSON, err := json.Marshal(wanted)
	if err != nil {
		return nil, err
	}

	attr := "nixTasksShellDrvs." + QuoteAttr(system)
	apply := fmt.Sprintf("drvs: builtins.intersectAttrs (builtins.fromJSON %s) drvs", NixString(string(wantedJSON)))
	var drvs map[string]string
	if err := e.eval(ctx, attr, []string{"--apply", apply}, &drvs); err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if drvs[t] == "" {
			return nil, fmt.Errorf("%s has no shell for task '%s'", attr, t)
		}
	}
	return drvs, nil
}

// PrintDevEnvCmd returns an exec.Cmd that prints the environment of the shell
// derivation drvPath as JSON (see ParseDevEnv). profile becomes a Nix profile
// pointing at the environment, which keeps it and the shell's dependencies
// alive across garbage collection for as long as the profile link exists.
func (e *Evaluator) PrintDevEnvCmd(ctx context.Context, drvPath, profile string) *exec.Cmd {
	args := []string{"print-dev-env", "--json", "--profile", profile, drvPath + "^*"}

	if e.debug {
		slog.Debug("running nix command", "cmd", "nix", "args", args)
	}

	return exec.CommandContext(ctx, "nix", args...)
}
