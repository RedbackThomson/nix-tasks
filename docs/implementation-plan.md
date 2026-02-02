# Nix-Tasks Implementation Plan

This document outlines a phased approach to building the Nix-based task runner system defined in [requirements.md](./requirements.md).

## Overview

The implementation is divided into **6 phases**, each delivering incremental value:

| Phase | Name | Goal | Key Deliverable |
|-------|------|------|-----------------|
| 1 | Foundation | Minimal viable task runner | Run simple tasks from Nix config |
| 2 | Task Orchestration | DAG execution with parallelism | Dependent tasks with parallel execution |
| 3 | Environment Management | Dev shells with inheritance | Multiple composable dev environments |
| 4 | Caching & Performance | Two-layer caching system | Fast incremental rebuilds |
| 5 | Developer Experience | CLI polish and TUI | Production-ready developer tooling |
| 6 | Standards & Migration | Company-wide distribution | Flake templates + Make compatibility |

## Technical Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Team familiarity, fast compilation, good CLI tooling |
| Module Path | `github.com/redbackthomson/nix-tasks` | |
| CLI Framework | [Kong](https://github.com/alecthomas/kong) | Clean struct-based API, good defaults |
| TUI Framework | [Bubbletea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) | Industry standard for Go TUIs |
| Nix Integration | Subprocess with clean abstraction | Battle-tested approach (used by devenv, etc.) |
| Store Path Handling | [go-nix](https://github.com/nix-community/go-nix) | Parse/validate store paths |
| Logging | `log/slog` (stdlib) | Standard library, structured logging |
| Errors | `errors` (stdlib) + `fmt.Errorf` wrapping | Standard Go 1.20+ patterns |
| Testing | `testing` (stdlib) + mocks for Nix | Unit tests (mocked) + integration tests (real Nix) |
| Cache Location | XDG (`~/.cache/nix-tasks/`) | Configurable via `NIX_TASKS_CACHE_DIR` |

---

## Phase 1: Foundation

**Goal:** Establish project structure and build a minimal task runner that can execute simple tasks defined in Nix.

### 1.1 Project Setup

#### Repository Structure
```
nix-tasks/
├── flake.nix                     # Project flake (builds the Go binary)
├── flake.lock
├── go.mod
├── go.sum
├── cmd/
│   └── nix-tasks/
│       └── main.go               # Entry point
├── internal/
│   ├── cli/                      # Kong CLI definitions
│   │   ├── cli.go                # Root CLI struct
│   │   ├── run.go                # run command
│   │   ├── list.go               # list command
│   │   └── describe.go           # describe command
│   ├── config/                   # Configuration types and loading
│   │   ├── config.go             # Config struct definitions
│   │   ├── loader.go             # Load config from Nix
│   │   └── validate.go           # Validation logic
│   ├── nix/                      # Nix integration layer
│   │   ├── evaluator.go          # Nix evaluation wrapper
│   │   ├── develop.go            # nix develop integration
│   │   ├── errors.go             # Nix-specific errors
│   │   └── store.go              # Store path utilities (go-nix)
│   ├── runner/                   # Task execution
│   │   ├── executor.go           # Execute single task
│   │   ├── script.go             # Generate shell scripts
│   │   └── output.go             # Output handling
│   └── ui/                       # Output formatting
│       ├── colors.go             # Color definitions
│       └── printer.go            # Status printing
├── lib/                          # Nix library
│   ├── default.nix               # Main entry point
│   ├── types.nix                 # Type definitions
│   ├── tasks.nix                 # Task evaluation
│   ├── shells.nix                # Shell generation
│   └── builders.nix              # Builder helpers
├── tests/
│   ├── unit/                     # Unit tests (mocked Nix)
│   └── integration/              # Integration tests (real Nix)
├── docs/
│   ├── requirements.md
│   └── implementation-plan.md
└── examples/
    ├── simple/                   # Minimal example
    └── go-service/               # Go project example
```

#### Flake Setup
```nix
# flake.nix
{
  description = "Nix-based task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "nix-tasks";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # Update after first build
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_22
            gopls
            golangci-lint
            nix # For integration tests
          ];
        };

        # Export the library for consumers
        lib = import ./lib { inherit pkgs; lib = pkgs.lib; };
      }
    );
}
```

#### Deliverables
- [ ] Initialize git repository
- [ ] Create `flake.nix` that builds the Go binary
- [ ] Set up `go.mod` with initial dependencies
- [ ] Create project directory structure
- [ ] Set up development shell with Go tooling
- [ ] Configure golangci-lint
- [ ] Write initial README

### 1.2 CLI Structure (Kong)

```go
// cmd/nix-tasks/main.go
package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/redbackthomson/nix-tasks/internal/cli"
)

func main() {
	var rootCmd cli.CLI
	ctx := kong.Parse(&rootCmd,
		kong.Name("nix-tasks"),
		kong.Description("Nix-based task runner"),
		kong.UsageOnError(),
	)
	err := ctx.Run(&rootCmd.Globals)
	ctx.FatalIfErrorf(err)
}
```

```go
// internal/cli/cli.go
package cli

import "github.com/redbackthomson/nix-tasks/internal/config"

// Globals contains flags available to all commands
type Globals struct {
	Verbose bool   `short:"v" help:"Show task output"`
	Debug   bool   `help:"Show debug information including Nix commands"`
	Flake   string `short:"f" help:"Path to flake" default:"."`
}

// CLI is the root command structure
type CLI struct {
	Globals

	Run      RunCmd      `cmd:"" help:"Run a task"`
	List     ListCmd     `cmd:"" help:"List available tasks"`
	Describe DescribeCmd `cmd:"" help:"Show task details"`
	Validate ValidateCmd `cmd:"" help:"Validate configuration"`
}
```

```go
// internal/cli/run.go
package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

type RunCmd struct {
	Task string `arg:"" help:"Task name to run"`
}

func (c *RunCmd) Run(globals *Globals) error {
	ctx := context.Background()

	// Initialize Nix evaluator
	eval := nix.NewEvaluator(globals.Flake)

	// Load configuration
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate task exists
	task, ok := cfg.Tasks[c.Task]
	if !ok {
		return fmt.Errorf("task not found: %s", c.Task)
	}

	// Execute task
	exec := runner.NewExecutor(eval, cfg, runner.ExecutorOptions{
		Verbose: globals.Verbose,
		Debug:   globals.Debug,
	})

	return exec.RunTask(ctx, c.Task, task)
}
```

```go
// internal/cli/list.go
package cli

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

type ListCmd struct{}

func (c *ListCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	printer := ui.NewPrinter()
	printer.PrintTaskList(cfg.Tasks)

	return nil
}
```

#### Deliverables
- [ ] Implement root CLI structure with Kong
- [ ] Implement `run` command (basic)
- [ ] Implement `list` command
- [ ] Implement `describe` command
- [ ] Implement `validate` command
- [ ] Add `--verbose` and `--debug` flags
- [ ] Add `--flake` flag for custom flake path

### 1.3 Nix Integration Layer

```go
// internal/nix/evaluator.go
package nix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

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
```

```go
// internal/nix/errors.go
package nix

import "fmt"

// EvalError represents a Nix evaluation error
type EvalError struct {
	Attribute string
	Stderr    string
	Err       error
}

func (e *EvalError) Error() string {
	return fmt.Sprintf("nix eval '%s' failed: %v\n%s", e.Attribute, e.Err, e.Stderr)
}

func (e *EvalError) Unwrap() error {
	return e.Err
}

// BuildError represents a Nix build error
type BuildError struct {
	Attribute string
	Stderr    string
	Err       error
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("nix build '%s' failed: %v\n%s", e.Attribute, e.Err, e.Stderr)
}

func (e *BuildError) Unwrap() error {
	return e.Err
}
```

```go
// internal/nix/store.go
package nix

import (
	"github.com/nix-community/go-nix/pkg/storepath"
)

// ParseStorePath validates and parses a Nix store path
func ParseStorePath(path string) (*storepath.StorePath, error) {
	return storepath.FromAbsolutePath(path)
}

// StorePathHash extracts the hash component from a store path
func StorePathHash(path string) (string, error) {
	sp, err := ParseStorePath(path)
	if err != nil {
		return "", err
	}
	return sp.Hash.String(), nil
}
```

#### Deliverables
- [ ] Implement `Evaluator` struct with `Eval`, `Build`, `DevelopCmd` methods
- [ ] Implement error types with helpful messages
- [ ] Add debug logging for Nix commands
- [ ] Integrate go-nix for store path handling
- [ ] Write unit tests with mocked exec

### 1.4 Configuration Types and Loading

```go
// internal/config/config.go
package config

// Config is the root configuration structure
type Config struct {
	Packages  map[string]string `json:"packages"`
	Tasks     map[string]Task   `json:"tasks"`
	DevShells map[string]Shell  `json:"devShells"`
}

// Task represents a single task definition
type Task struct {
	Description string            `json:"description"`
	Deps        []string          `json:"deps"`
	Depends     []string          `json:"depends"`
	Commands    []string          `json:"commands"`
	Script      string            `json:"script"`
	Env         map[string]string `json:"env"`
	WorkingDir  string            `json:"workingDir"`
	Inputs      []string          `json:"inputs"`
	Outputs     []string          `json:"outputs"`

	// Error handling
	ContinueOnError bool `json:"continueOnError"`
}

// Shell represents a dev shell definition
type Shell struct {
	Extends   string            `json:"extends"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
	ShellHook string            `json:"shellHook"`
}
```

```go
// internal/config/loader.go
package config

import (
	"context"
	"fmt"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// Load evaluates the Nix configuration and returns the parsed config
func Load(ctx context.Context, eval *nix.Evaluator) (*Config, error) {
	var cfg Config

	// Try nixTasksConfig first (flake output)
	err := eval.Eval(ctx, "nixTasksConfig", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate nixTasksConfig: %w", err)
	}

	// Validate the loaded config
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
```

```go
// internal/config/validate.go
package config

import (
	"fmt"
	"strings"
)

// ValidationError contains all validation errors found
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed:\n  %s", strings.Join(e.Errors, "\n  "))
}

// Validate checks the configuration for errors
func Validate(cfg *Config) error {
	var errs []string

	// Check task deps reference valid packages
	for taskName, task := range cfg.Tasks {
		for _, dep := range task.Deps {
			if _, ok := cfg.Packages[dep]; !ok {
				errs = append(errs, fmt.Sprintf("task '%s': unknown package '%s'", taskName, dep))
			}
		}

		// Check task depends reference valid tasks
		for _, dep := range task.Depends {
			depName := strings.TrimPrefix(dep, "task:")
			if _, ok := cfg.Tasks[depName]; !ok {
				errs = append(errs, fmt.Sprintf("task '%s': unknown task dependency '%s'", taskName, dep))
			}
		}
	}

	// Check shell packages reference valid packages
	for shellName, shell := range cfg.DevShells {
		for _, pkg := range shell.Packages {
			if _, ok := cfg.Packages[pkg]; !ok {
				errs = append(errs, fmt.Sprintf("shell '%s': unknown package '%s'", shellName, pkg))
			}
		}

		// Check extends references valid shell
		if shell.Extends != "" {
			if _, ok := cfg.DevShells[shell.Extends]; !ok {
				errs = append(errs, fmt.Sprintf("shell '%s': unknown parent shell '%s'", shellName, shell.Extends))
			}
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}
```

#### Deliverables
- [ ] Define config structs matching Nix schema
- [ ] Implement config loading from Nix evaluation
- [ ] Implement validation with clear error messages
- [ ] Write unit tests for validation logic

### 1.5 Task Execution

```go
// internal/runner/executor.go
package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ExecutorOptions configures task execution
type ExecutorOptions struct {
	Verbose bool
	Debug   bool
}

// Executor runs tasks
type Executor struct {
	nix     *nix.Evaluator
	config  *config.Config
	options ExecutorOptions
	printer *ui.Printer
}

// NewExecutor creates a new task executor
func NewExecutor(eval *nix.Evaluator, cfg *config.Config, opts ExecutorOptions) *Executor {
	eval.SetDebug(opts.Debug)
	return &Executor{
		nix:     eval,
		config:  cfg,
		options: opts,
		printer: ui.NewPrinter(),
	}
}

// RunTask executes a single task
func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
	e.printer.TaskStarted(name)

	// Generate the script to run
	script := GenerateScript(task)

	// Run inside nix develop with the task's shell
	shellAttr := fmt.Sprintf("nixTasksShells.%s", name)
	cmd := e.nix.DevelopCmd(ctx, shellAttr, []string{"bash", "-e", "-c", script})

	// Configure output
	if e.options.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// Buffer output, only show on error
		cmd.Stdout = e.printer.TaskBuffer(name)
		cmd.Stderr = e.printer.TaskBuffer(name)
	}

	// Set working directory if specified
	if task.WorkingDir != "" {
		cmd.Dir = task.WorkingDir
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute
	err := cmd.Run()
	if err != nil {
		e.printer.TaskFailed(name, err)
		return fmt.Errorf("task '%s' failed: %w", name, err)
	}

	e.printer.TaskSucceeded(name)
	return nil
}
```

```go
// internal/runner/script.go
package runner

import (
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// GenerateScript creates a bash script from task definition
func GenerateScript(task config.Task) string {
	// If raw script is provided, use it directly
	if task.Script != "" {
		return task.Script
	}

	// Otherwise, join commands with newlines
	return strings.Join(task.Commands, "\n")
}
```

```go
// internal/ui/printer.go
package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Printer handles formatted output
type Printer struct {
	buffers map[string]*bytes.Buffer
}

// NewPrinter creates a new printer
func NewPrinter() *Printer {
	return &Printer{
		buffers: make(map[string]*bytes.Buffer),
	}
}

// TaskBuffer returns a buffer for capturing task output
func (p *Printer) TaskBuffer(name string) io.Writer {
	buf := &bytes.Buffer{}
	p.buffers[name] = buf
	return buf
}

// TaskStarted prints task start message
func (p *Printer) TaskStarted(name string) {
	// In non-verbose mode, we don't print anything on start
}

// TaskSucceeded prints success message
func (p *Printer) TaskSucceeded(name string) {
	fmt.Fprintf(os.Stdout, "%s %s\n", Green("✓"), name)
}

// TaskFailed prints failure message and buffered output
func (p *Printer) TaskFailed(name string, err error) {
	fmt.Fprintf(os.Stdout, "%s %s\n", Red("✗"), name)

	// Print buffered output if we have it
	if buf, ok := p.buffers[name]; ok && buf.Len() > 0 {
		fmt.Fprintf(os.Stdout, "%s\n", buf.String())
	}
}

// PrintTaskList prints a formatted list of tasks
func (p *Printer) PrintTaskList(tasks map[string]config.Task) {
	fmt.Println("Tasks:")
	for name, task := range tasks {
		if task.Description != "" {
			fmt.Printf("  %-20s %s\n", name, task.Description)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}
```

```go
// internal/ui/colors.go
package ui

import "fmt"

// ANSI color codes
const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
)

// Green returns text in green
func Green(s string) string {
	return fmt.Sprintf("%s%s%s", colorGreen, s, colorReset)
}

// Red returns text in red
func Red(s string) string {
	return fmt.Sprintf("%s%s%s", colorRed, s, colorReset)
}
```

#### Deliverables
- [ ] Implement task executor
- [ ] Implement script generation from commands
- [ ] Implement output handling (verbose vs buffered)
- [ ] Implement colored status output (green success, red failure)
- [ ] Support task environment variables
- [ ] Support working directory

### 1.6 Nix Library

```nix
# lib/default.nix
{ pkgs, lib }:
let
  types = import ./types.nix { inherit lib; };
  builders = import ./builders.nix { inherit lib pkgs; };

  # Evaluate user config and generate task shells
  evalConfig = userConfig:
    let
      validated = types.validateConfig userConfig;
      taskShells = import ./shells.nix {
        inherit lib pkgs;
        config = validated;
      };
    in {
      # Config for JSON export to Go
      nixTasksConfig = {
        packages = lib.mapAttrs (name: pkg: pkg.outPath or pkg) validated.packages;
        tasks = validated.tasks;
        devShells = validated.devShells;
      };

      # Generated shells for task execution
      nixTasksShells = taskShells.taskShells;

      # User-facing dev shells
      devShells = taskShells.devShells;
    };
in {
  inherit evalConfig types builders;

  # Convenience: mkTask and other builders
  inherit (builders) mkTask mkGoTask mkDockerTask mkCompoundTask;
}
```

```nix
# lib/types.nix
{ lib }:
{
  # Task type definition
  taskType = {
    description ? "",
    deps ? [],
    depends ? [],
    commands ? [],
    script ? null,
    env ? {},
    workingDir ? null,
    inputs ? [],
    outputs ? [],
    continueOnError ? false,
  }: {
    inherit description deps depends commands script env workingDir inputs outputs continueOnError;
  };

  # Shell type definition
  shellType = {
    extends ? null,
    packages ? [],
    env ? {},
    shellHook ? "",
  }: {
    inherit extends packages env shellHook;
  };

  # Validate configuration
  validateConfig = config:
    let
      # Ensure required fields exist
      packages = config.packages or {};
      tasks = config.tasks or {};
      devShells = config.devShells or {};
    in {
      inherit packages tasks devShells;
    };
}
```

```nix
# lib/shells.nix
{ lib, pkgs, config }:
let
  # Resolve shell inheritance
  resolveShell = name: shell:
    let
      parent = if shell.extends != null
        then resolveShell shell.extends config.devShells.${shell.extends}
        else { packages = []; env = {}; shellHook = ""; };

      resolvedPackages = parent.packages ++
        (map (p: config.packages.${p}) shell.packages);
      resolvedEnv = parent.env // shell.env;
      resolvedHook = parent.shellHook + "\n" + shell.shellHook;
    in {
      packages = resolvedPackages;
      env = resolvedEnv;
      shellHook = resolvedHook;
    };

  # Create mkShell from resolved shell
  mkDevShell = name: shell:
    let
      resolved = resolveShell name shell;
    in pkgs.mkShell {
      packages = resolved.packages;
      shellHook = resolved.shellHook;
    } // {
      # Add env vars
      env = resolved.env;
    };

  # Create minimal shell for a task (just its deps)
  mkTaskShell = name: task:
    pkgs.mkShell {
      packages = map (dep: config.packages.${dep}) task.deps;
    };

in {
  # User-facing dev shells
  devShells = lib.mapAttrs mkDevShell config.devShells;

  # Task-specific minimal shells
  taskShells = lib.mapAttrs mkTaskShell config.tasks;
}
```

```nix
# lib/builders.nix
{ lib, pkgs }:
rec {
  # Generic task builder
  mkTask = {
    name ? null,
    description ? "",
    deps ? [],
    depends ? [],
    commands ? [],
    script ? null,
    env ? {},
    workingDir ? null,
    inputs ? [],
    outputs ? [],
    continueOnError ? false,
    ...
  }: {
    inherit description deps depends commands script env workingDir inputs outputs continueOnError;
  };

  # Go-specific task builder
  mkGoTask = {
    name ? null,
    description ? "",
    command ? "build",
    output ? null,
    packages ? [],
    ldflags ? "",
    deps ? [],
    ...
  }@args:
    mkTask ({
      inherit description;
      deps = ["go"] ++ packages ++ deps;
      commands = [
        ("go ${command}" +
          lib.optionalString (ldflags != "") " -ldflags '${ldflags}'" +
          lib.optionalString (output != null) " -o ${output}" +
          " ./...")
      ];
    } // (removeAttrs args ["command" "output" "packages" "ldflags"]));

  # Docker task builder
  mkDockerTask = {
    name ? null,
    description ? "",
    image,
    tag ? "latest",
    context ? ".",
    dockerfile ? "Dockerfile",
    deps ? [],
    ...
  }@args:
    mkTask ({
      inherit description;
      deps = ["docker"] ++ deps;
      commands = [
        "docker build -t ${image}:${tag} -f ${dockerfile} ${context}"
      ];
    } // (removeAttrs args ["image" "tag" "context" "dockerfile"]));

  # Compound task (groups other tasks)
  mkCompoundTask = {
    name ? null,
    description ? "",
    tasks,
    ...
  }@args:
    mkTask ({
      inherit description;
      depends = map (t: "task:${t}") tasks;
      commands = [];
    } // (removeAttrs args ["tasks"]));
}
```

#### Deliverables
- [ ] Implement `lib/default.nix` entry point
- [ ] Implement type definitions and validation
- [ ] Implement shell inheritance resolution
- [ ] Implement per-task shell generation
- [ ] Implement builder helpers (`mkTask`, `mkGoTask`, `mkDockerTask`, `mkCompoundTask`)
- [ ] Write tests for Nix library

### Phase 1 Milestone

**Demo:** User can define tasks in a `flake.nix`, run `nix-tasks list` to see them, and `nix-tasks run <task>` to execute them.

```nix
# Example flake.nix at end of Phase 1
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
      lib = nix-tasks.lib;

      config = lib.evalConfig {
        packages = {
          go = pkgs.go;
          docker = pkgs.docker;
        };

        tasks = {
          build = lib.mkGoTask {
            description = "Build the application";
            output = "bin/app";
          };

          test = lib.mkTask {
            description = "Run tests";
            deps = ["go"];
            commands = ["go test ./..."];
          };
        };

        devShells = {
          default = {
            packages = ["go" "docker"];
            shellHook = ''
              echo "Development shell ready"
            '';
          };
        };
      };
    in config // {
      devShells.${system} = config.devShells;
    };
}
```

```bash
$ nix-tasks list
Tasks:
  build                Build the application
  test                 Run tests

$ nix-tasks run build
✓ build

$ nix-tasks run test
✓ test
```

---

## Phase 2: Task Orchestration

**Goal:** Build dependency graph, execute tasks in correct order with parallelism, implement error handling strategies.

### 2.1 Dependency Graph

```go
// internal/runner/graph.go
package runner

import (
	"fmt"
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// TaskGraph represents the task dependency DAG
type TaskGraph struct {
	tasks    map[string]config.Task
	edges    map[string][]string // task -> tasks it depends on
	reverse  map[string][]string // task -> tasks that depend on it
}

// NewTaskGraph builds a dependency graph from configuration
func NewTaskGraph(tasks map[string]config.Task) (*TaskGraph, error) {
	g := &TaskGraph{
		tasks:   tasks,
		edges:   make(map[string][]string),
		reverse: make(map[string][]string),
	}

	// Build edges
	for name, task := range tasks {
		g.edges[name] = []string{}
		for _, dep := range task.Depends {
			depName := strings.TrimPrefix(dep, "task:")
			if _, ok := tasks[depName]; !ok {
				return nil, fmt.Errorf("task '%s' depends on unknown task '%s'", name, depName)
			}
			g.edges[name] = append(g.edges[name], depName)
			g.reverse[depName] = append(g.reverse[depName], name)
		}
	}

	// Check for cycles
	if cycle := g.findCycle(); cycle != nil {
		return nil, fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " -> "))
	}

	return g, nil
}

// findCycle returns a cycle if one exists, nil otherwise
func (g *TaskGraph) findCycle() []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string, path []string) []string
	dfs = func(node string, path []string) []string {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, dep := range g.edges[node] {
			if !visited[dep] {
				if cycle := dfs(dep, path); cycle != nil {
					return cycle
				}
			} else if recStack[dep] {
				// Found cycle
				return append(path, dep)
			}
		}

		recStack[node] = false
		return nil
	}

	for name := range g.tasks {
		if !visited[name] {
			if cycle := dfs(name, nil); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// ExecutionOrder returns tasks in topological order for execution
func (g *TaskGraph) ExecutionOrder(target string) ([]string, error) {
	visited := make(map[string]bool)
	var order []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true

		for _, dep := range g.edges[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		order = append(order, name)
		return nil
	}

	if err := visit(target); err != nil {
		return nil, err
	}

	return order, nil
}

// ParallelGroups returns tasks grouped by depth level for parallel execution
func (g *TaskGraph) ParallelGroups(target string) ([][]string, error) {
	order, err := g.ExecutionOrder(target)
	if err != nil {
		return nil, err
	}

	// Compute depth for each task
	depth := make(map[string]int)
	for _, name := range order {
		maxDepth := 0
		for _, dep := range g.edges[name] {
			if depth[dep] >= maxDepth {
				maxDepth = depth[dep] + 1
			}
		}
		depth[name] = maxDepth
	}

	// Group by depth
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}

	groups := make([][]string, maxDepth+1)
	for _, name := range order {
		d := depth[name]
		groups[d] = append(groups[d], name)
	}

	return groups, nil
}
```

#### Deliverables
- [ ] Implement DAG construction from task dependencies
- [ ] Implement cycle detection with clear error messages
- [ ] Implement topological sort for execution order
- [ ] Implement parallel grouping by depth level
- [ ] Write unit tests for graph operations

### 2.2 Parallel Execution

```go
// internal/runner/parallel.go
package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// ParallelExecutor runs tasks with parallelism
type ParallelExecutor struct {
	executor *Executor
	maxJobs  int
}

// NewParallelExecutor creates a parallel executor
func NewParallelExecutor(exec *Executor, maxJobs int) *ParallelExecutor {
	if maxJobs <= 0 {
		maxJobs = 4 // Default parallelism
	}
	return &ParallelExecutor{
		executor: exec,
		maxJobs:  maxJobs,
	}
}

// TaskResult holds the result of a task execution
type TaskResult struct {
	Name    string
	Success bool
	Error   error
}

// ExecuteDAG executes all tasks required for target, respecting dependencies
func (p *ParallelExecutor) ExecuteDAG(ctx context.Context, graph *TaskGraph, target string, strategy FailureStrategy) ([]TaskResult, error) {
	groups, err := graph.ParallelGroups(target)
	if err != nil {
		return nil, err
	}

	var results []TaskResult
	var failed bool

	for _, group := range groups {
		if failed && strategy == FailFast {
			// Skip remaining groups on failure
			break
		}

		groupResults := p.executeGroup(ctx, group, strategy)
		results = append(results, groupResults...)

		// Check for failures
		for _, r := range groupResults {
			if !r.Success {
				failed = true
				if strategy == FailFast {
					break
				}
			}
		}
	}

	return results, nil
}

// executeGroup runs a group of independent tasks in parallel
func (p *ParallelExecutor) executeGroup(ctx context.Context, tasks []string, strategy FailureStrategy) []TaskResult {
	results := make([]TaskResult, len(tasks))
	sem := make(chan struct{}, p.maxJobs)
	var wg sync.WaitGroup

	for i, name := range tasks {
		wg.Add(1)
		go func(idx int, taskName string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check if context cancelled
			if ctx.Err() != nil {
				results[idx] = TaskResult{
					Name:    taskName,
					Success: false,
					Error:   ctx.Err(),
				}
				return
			}

			// Execute task
			task := p.executor.config.Tasks[taskName]
			err := p.executor.RunTask(ctx, taskName, task)

			results[idx] = TaskResult{
				Name:    taskName,
				Success: err == nil,
				Error:   err,
			}
		}(i, name)
	}

	wg.Wait()
	return results
}
```

```go
// internal/runner/strategy.go
package runner

// FailureStrategy defines how to handle task failures
type FailureStrategy int

const (
	// FailFast stops execution on first failure
	FailFast FailureStrategy = iota
	// ContinueOnError continues with independent tasks
	ContinueOnError
)
```

#### Deliverables
- [ ] Implement parallel task execution with goroutines
- [ ] Implement semaphore-based concurrency limiting (`--jobs N`)
- [ ] Implement execution by dependency group
- [ ] Support context cancellation
- [ ] Write integration tests for parallel execution

### 2.3 Error Handling Strategies

```go
// internal/cli/run.go (updated)
package cli

type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            int    `short:"j" help:"Number of parallel jobs" default:"4"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
}

func (c *RunCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Build dependency graph
	graph, err := runner.NewTaskGraph(cfg.Tasks)
	if err != nil {
		return err
	}

	// Determine strategy
	strategy := runner.FailFast
	if c.ContinueOnError {
		strategy = runner.ContinueOnError
	}

	// Execute
	exec := runner.NewExecutor(eval, cfg, runner.ExecutorOptions{
		Verbose: globals.Verbose,
		Debug:   globals.Debug,
	})
	parallel := runner.NewParallelExecutor(exec, c.Jobs)

	results, err := parallel.ExecuteDAG(ctx, graph, c.Task, strategy)

	// Print summary
	printSummary(results)

	// Return error if any task failed
	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("one or more tasks failed")
		}
	}

	return nil
}

func printSummary(results []TaskResult) {
	var passed, failed int
	for _, r := range results {
		if r.Success {
			passed++
		} else {
			failed++
		}
	}

	fmt.Printf("\nCompleted: %d tasks (%d passed", len(results), passed)
	if failed > 0 {
		fmt.Printf(", %s", ui.Red(fmt.Sprintf("%d failed", failed)))
	}
	fmt.Println(")")
}
```

#### Deliverables
- [ ] Implement `--continue-on-error` flag
- [ ] Per-task `continueOnError` support (already in schema)
- [ ] Print execution summary with pass/fail counts
- [ ] Return appropriate exit code based on results

### 2.4 Output Management

```go
// internal/runner/output.go
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// OutputMode determines how task output is displayed
type OutputMode int

const (
	// Buffered collects output, shows on error (default for local)
	Buffered OutputMode = iota
	// Streaming shows output in real-time with prefixes (default for CI)
	Streaming
)

// DetectOutputMode returns appropriate mode based on environment
func DetectOutputMode() OutputMode {
	if os.Getenv("CI") != "" {
		return Streaming
	}
	return Buffered
}

// OutputManager handles task output based on mode
type OutputManager struct {
	mode    OutputMode
	mu      sync.Mutex
	buffers map[string]*bytes.Buffer
}

// NewOutputManager creates an output manager
func NewOutputManager(mode OutputMode) *OutputManager {
	return &OutputManager{
		mode:    mode,
		buffers: make(map[string]*bytes.Buffer),
	}
}

// Writer returns an io.Writer for task output
func (m *OutputManager) Writer(taskName string) io.Writer {
	switch m.mode {
	case Streaming:
		return &prefixWriter{
			prefix: fmt.Sprintf("[%s] ", taskName),
			out:    os.Stdout,
			mu:     &m.mu,
		}
	case Buffered:
		buf := &bytes.Buffer{}
		m.mu.Lock()
		m.buffers[taskName] = buf
		m.mu.Unlock()
		return buf
	default:
		return io.Discard
	}
}

// FlushOnError prints buffered output for a failed task
func (m *OutputManager) FlushOnError(taskName string) {
	if m.mode != Buffered {
		return
	}

	m.mu.Lock()
	buf, ok := m.buffers[taskName]
	m.mu.Unlock()

	if ok && buf.Len() > 0 {
		fmt.Print(buf.String())
	}
}

// prefixWriter adds a prefix to each line
type prefixWriter struct {
	prefix string
	out    io.Writer
	mu     *sync.Mutex
}

func (w *prefixWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines := bytes.Split(p, []byte("\n"))
	for i, line := range lines {
		if len(line) > 0 || i < len(lines)-1 {
			fmt.Fprintf(w.out, "%s%s\n", w.prefix, line)
		}
	}
	return len(p), nil
}
```

#### Deliverables
- [ ] Implement `OutputManager` with buffered/streaming modes
- [ ] Auto-detect CI environment for streaming mode
- [ ] Add `--stream` flag to force streaming
- [ ] Prefix output lines with task name in streaming mode
- [ ] Buffer output and show on failure in buffered mode

### Phase 2 Milestone

**Demo:** User can define tasks with dependencies, run `nix-tasks run deploy` and see dependent tasks execute in parallel where possible.

```bash
$ nix-tasks run deploy
✓ build (2.3s)
✓ test (4.1s)      # Runs in parallel with lint
✗ lint (1.2s)
  src/main.go:42: ineffassign: variable 'err' is not used
✓ deploy (0.8s)

Completed: 4 tasks (3 passed, 1 failed)

$ nix-tasks run deploy --continue-on-error
# Continues even if lint fails
```

---

## Phase 3: Environment Management

**Goal:** Implement dev shells with composition/inheritance, ensure version consistency between tasks and shells.

### 3.1 Shell Resolution

Update the Nix library to properly resolve shell inheritance:

```nix
# lib/shells.nix (updated)
{ lib, pkgs, config }:
let
  # Detect circular inheritance
  checkCircular = name: visited:
    if builtins.elem name visited
    then throw "Circular shell inheritance: ${builtins.concatStringsSep " -> " (visited ++ [name])}"
    else visited ++ [name];

  # Resolve shell inheritance chain
  resolveShell = name: shell: visited:
    let
      newVisited = checkCircular name visited;
      parent = if shell.extends != null
        then resolveShell shell.extends config.devShells.${shell.extends} newVisited
        else { packages = []; env = {}; shellHook = ""; };

      resolvedPackages = parent.packages ++
        (map (p: config.packages.${p}) shell.packages);
      resolvedEnv = parent.env // shell.env;
      resolvedHook = lib.strings.concatStringsSep "\n" (
        lib.filter (s: s != "") [parent.shellHook shell.shellHook]
      );
    in {
      packages = resolvedPackages;
      env = resolvedEnv;
      shellHook = resolvedHook;
    };

  # Create mkShell from resolved shell
  mkDevShell = name: shell:
    let
      resolved = resolveShell name shell [];
    in pkgs.mkShell ({
      packages = resolved.packages;
      shellHook = resolved.shellHook;
    } // lib.mapAttrs' (k: v: lib.nameValuePair k v) resolved.env);

  # Create minimal shell for a task (just its deps)
  mkTaskShell = name: task:
    pkgs.mkShell {
      packages = map (dep: config.packages.${dep}) (task.deps or []);
    };

in {
  devShells = lib.mapAttrs mkDevShell (config.devShells or {});
  taskShells = lib.mapAttrs mkTaskShell (config.tasks or {});
}
```

### 3.2 Shell Command

```go
// internal/cli/shell.go
package cli

type ShellCmd struct {
	Name string `arg:"" optional:"" help:"Shell name (default: default)"`
}

func (c *ShellCmd) Run(globals *Globals) error {
	ctx := context.Background()

	shellName := c.Name
	if shellName == "" {
		shellName = "default"
	}

	eval := nix.NewEvaluator(globals.Flake)

	// Verify shell exists
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return err
	}

	if _, ok := cfg.DevShells[shellName]; !ok {
		return fmt.Errorf("shell not found: %s", shellName)
	}

	// Launch interactive shell
	shellAttr := fmt.Sprintf("devShells.%s", shellName)
	cmd := exec.CommandContext(ctx, "nix", "develop",
		fmt.Sprintf("%s#%s", globals.Flake, shellAttr))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
```

```go
// internal/cli/cli.go (add shell command)
type CLI struct {
	Globals

	Run      RunCmd      `cmd:"" help:"Run a task"`
	List     ListCmd     `cmd:"" help:"List available tasks"`
	Describe DescribeCmd `cmd:"" help:"Show task details"`
	Validate ValidateCmd `cmd:"" help:"Validate configuration"`
	Shell    ShellCmd    `cmd:"" help:"Enter a development shell"`
}
```

#### Deliverables
- [ ] Update Nix library for proper shell inheritance
- [ ] Detect and report circular inheritance
- [ ] Add `shell` command to enter dev shells
- [ ] Default to `default` shell if no name provided
- [ ] List available shells in `list` command

### Phase 3 Milestone

**Demo:** User can define multiple shells with inheritance and enter them.

```nix
devShells = {
  minimal = {
    packages = ["go"];
  };
  ci = {
    extends = "minimal";
    packages = ["docker"];
  };
  default = {
    extends = "ci";
    packages = ["kubectl" "k9s"];
    shellHook = ''
      alias k=kubectl
    '';
  };
};
```

```bash
$ nix-tasks shell ci
# Enters shell with go + docker

$ nix-tasks shell
# Enters default shell with go + docker + kubectl + k9s
```

---

## Phase 4: Caching & Performance

**Goal:** Implement task fingerprinting and local caching with force rebuild option.

### 4.1 Cache Location

```go
// internal/cache/location.go
package cache

import (
	"os"
	"path/filepath"
)

// Dir returns the cache directory
func Dir() string {
	// Check environment variable first
	if dir := os.Getenv("NIX_TASKS_CACHE_DIR"); dir != "" {
		return dir
	}

	// Use XDG cache directory
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "nix-tasks")
	}

	// Fall back to ~/.cache/nix-tasks
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "nix-tasks")
}
```

### 4.2 Fingerprinting

```go
// internal/cache/fingerprint.go
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// Fingerprint represents a content-based hash of task inputs
type Fingerprint struct {
	Hash string
}

// ComputeFingerprint calculates the fingerprint for a task
func ComputeFingerprint(task config.Task, packages map[string]string, workDir string) (*Fingerprint, error) {
	h := sha256.New()

	// Hash task definition (excluding description)
	taskData := struct {
		Deps     []string
		Commands []string
		Script   string
		Env      map[string]string
	}{
		Deps:     task.Deps,
		Commands: task.Commands,
		Script:   task.Script,
		Env:      task.Env,
	}
	taskJSON, _ := json.Marshal(taskData)
	h.Write(taskJSON)

	// Hash package store paths (sorted for determinism)
	deps := make([]string, 0, len(task.Deps))
	for _, dep := range task.Deps {
		if path, ok := packages[dep]; ok {
			deps = append(deps, path)
		}
	}
	sort.Strings(deps)
	for _, path := range deps {
		h.Write([]byte(path))
	}

	// Hash input files if specified
	if len(task.Inputs) > 0 {
		if err := hashInputFiles(h, workDir, task.Inputs); err != nil {
			return nil, err
		}
	}

	return &Fingerprint{
		Hash: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func hashInputFiles(h io.Writer, workDir string, patterns []string) error {
	var files []string

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err != nil {
			return err
		}
		files = append(files, matches...)
	}

	// Sort for determinism
	sort.Strings(files)

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}

		// Write relative path
		rel, _ := filepath.Rel(workDir, file)
		h.Write([]byte(rel))

		// Write file content
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		io.Copy(h, f)
		f.Close()
	}

	return nil
}
```

### 4.3 Cache Store

```go
// internal/cache/store.go
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a cached task result
type Entry struct {
	Fingerprint string    `json:"fingerprint"`
	Success     bool      `json:"success"`
	Timestamp   time.Time `json:"timestamp"`
}

// Store manages the task result cache
type Store struct {
	dir        string
	projectKey string
}

// NewStore creates a cache store for a project
func NewStore(projectKey string) *Store {
	return &Store{
		dir:        Dir(),
		projectKey: projectKey,
	}
}

func (s *Store) entryPath(taskName string, fp *Fingerprint) string {
	return filepath.Join(s.dir, s.projectKey, taskName, fp.Hash+".json")
}

// Lookup checks if a cached result exists
func (s *Store) Lookup(taskName string, fp *Fingerprint) (*Entry, bool) {
	path := s.entryPath(taskName, fp)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Verify fingerprint matches
	if entry.Fingerprint != fp.Hash {
		return nil, false
	}

	return &entry, true
}

// Store saves a task result to cache
func (s *Store) Store(taskName string, fp *Fingerprint, success bool) error {
	path := s.entryPath(taskName, fp)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	entry := Entry{
		Fingerprint: fp.Hash,
		Success:     success,
		Timestamp:   time.Now(),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Clear removes all cached entries for this project
func (s *Store) Clear() error {
	return os.RemoveAll(filepath.Join(s.dir, s.projectKey))
}
```

### 4.4 Integration with Executor

```go
// internal/runner/executor.go (updated)

type ExecutorOptions struct {
	Verbose    bool
	Debug      bool
	Force      bool   // Bypass cache
	NoCache    bool   // Don't read or write cache
	ProjectKey string // Cache key for project
}

func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
	// Compute fingerprint
	fp, err := cache.ComputeFingerprint(task, e.config.Packages, e.workDir)
	if err != nil {
		return fmt.Errorf("failed to compute fingerprint: %w", err)
	}

	// Check cache
	if !e.options.Force && !e.options.NoCache {
		if entry, ok := e.cache.Lookup(name, fp); ok {
			e.printer.TaskCached(name)
			if !entry.Success {
				return fmt.Errorf("task '%s' failed (cached)", name)
			}
			return nil
		}
	}

	// Execute task
	e.printer.TaskStarted(name)
	err = e.executeTask(ctx, name, task)

	// Store result in cache
	if !e.options.NoCache {
		e.cache.Store(name, fp, err == nil)
	}

	if err != nil {
		e.printer.TaskFailed(name, err)
		return err
	}

	e.printer.TaskSucceeded(name)
	return nil
}
```

### 4.5 Cache Commands

```go
// internal/cli/cache.go
package cli

type CacheCmd struct {
	Clean CleanCacheCmd `cmd:"" help:"Clear the cache"`
	Stats StatsCacheCmd `cmd:"" help:"Show cache statistics"`
}

type CleanCacheCmd struct{}

func (c *CleanCacheCmd) Run(globals *Globals) error {
	store := cache.NewStore(projectKey(globals.Flake))
	if err := store.Clear(); err != nil {
		return err
	}
	fmt.Println("Cache cleared")
	return nil
}

type StatsCacheCmd struct{}

func (c *StatsCacheCmd) Run(globals *Globals) error {
	// Show cache size, entry count, etc.
	// Implementation details...
	return nil
}
```

#### Deliverables
- [ ] Implement XDG-based cache location with env var override
- [ ] Implement content-based fingerprinting
- [ ] Implement cache store with JSON entries
- [ ] Integrate caching into executor
- [ ] Add `--force` flag to bypass cache
- [ ] Add `--no-cache` flag to disable caching
- [ ] Add `cache clean` command
- [ ] Add `cache stats` command
- [ ] Show "(cached)" indicator for cache hits

### Phase 4 Milestone

```bash
$ nix-tasks run build
✓ build (3.2s)

$ nix-tasks run build
✓ build (cached)

$ touch cmd/app/main.go
$ nix-tasks run build
✓ build (3.1s)

$ nix-tasks run build --force
✓ build (3.2s, forced)

$ nix-tasks cache clean
Cache cleared
```

---

## Phase 5: Developer Experience

**Goal:** Polish CLI output, implement interactive TUI.

### 5.1 CLI Polish

```go
// internal/cli/describe.go
package cli

type DescribeCmd struct {
	Task string `arg:"" help:"Task name to describe"`
}

func (c *DescribeCmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return err
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
```

### 5.2 Interactive TUI

```go
// internal/tui/app.go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/redbackthomson/nix-tasks/internal/config"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))
)

type Model struct {
	config      *config.Config
	tasks       []string
	shells      []string
	cursor      int
	mode        mode
	width       int
	height      int
}

type mode int

const (
	taskMode mode = iota
	shellMode
)

func NewModel(cfg *config.Config) Model {
	tasks := make([]string, 0, len(cfg.Tasks))
	for name := range cfg.Tasks {
		tasks = append(tasks, name)
	}
	sort.Strings(tasks)

	shells := make([]string, 0, len(cfg.DevShells))
	for name := range cfg.DevShells {
		shells = append(shells, name)
	}
	sort.Strings(shells)

	return Model{
		config: cfg,
		tasks:  tasks,
		shells: shells,
		mode:   taskMode,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			max := len(m.tasks) - 1
			if m.mode == shellMode {
				max = len(m.shells) - 1
			}
			if m.cursor < max {
				m.cursor++
			}
		case "tab":
			if m.mode == taskMode {
				m.mode = shellMode
			} else {
				m.mode = taskMode
			}
			m.cursor = 0
		case "enter", "r":
			return m, m.runSelected()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	var s string

	// Title
	s += titleStyle.Render("nix-tasks") + "\n\n"

	// Tasks section
	if m.mode == taskMode {
		s += titleStyle.Render("Tasks") + "\n"
	} else {
		s += normalStyle.Render("Tasks") + "\n"
	}

	for i, task := range m.tasks {
		cursor := "  "
		style := normalStyle
		if m.mode == taskMode && i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}
		desc := m.config.Tasks[task].Description
		if desc != "" {
			s += style.Render(fmt.Sprintf("%s%-15s %s", cursor, task, desc)) + "\n"
		} else {
			s += style.Render(fmt.Sprintf("%s%s", cursor, task)) + "\n"
		}
	}

	s += "\n"

	// Shells section
	if m.mode == shellMode {
		s += titleStyle.Render("Dev Shells") + "\n"
	} else {
		s += normalStyle.Render("Dev Shells") + "\n"
	}

	for i, shell := range m.shells {
		cursor := "  "
		style := normalStyle
		if m.mode == shellMode && i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}
		s += style.Render(fmt.Sprintf("%s%s", cursor, shell)) + "\n"
	}

	s += "\n"
	s += normalStyle.Render("[r]un  [tab] switch  [q]uit")

	return s
}

func (m Model) runSelected() tea.Cmd {
	return tea.ExecProcess(
		exec.Command("nix-tasks", "run", m.tasks[m.cursor]),
		nil,
	)
}
```

```go
// internal/cli/tui.go
package cli

type TUICmd struct{}

func (c *TUICmd) Run(globals *Globals) error {
	ctx := context.Background()

	eval := nix.NewEvaluator(globals.Flake)
	cfg, err := config.Load(ctx, eval)
	if err != nil {
		return err
	}

	model := tui.NewModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
```

#### Deliverables
- [ ] Enhanced `describe` command with full details
- [ ] `list` shows both tasks and shells
- [ ] Implement TUI with bubbletea
- [ ] Keyboard navigation (j/k, arrows)
- [ ] Run tasks from TUI
- [ ] Tab to switch between tasks/shells
- [ ] Launch shell from TUI

### 5.3 Shell Completions (Stretch Goal)

```go
// internal/cli/completions.go
package cli

import (
	"github.com/alecthomas/kong"
)

type CompletionsCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish" help:"Shell type"`
}

func (c *CompletionsCmd) Run(globals *Globals) error {
	// Kong has built-in completion support
	// Generate completions for the specified shell
	return nil
}
```

#### Deliverables (Stretch)
- [ ] `completions` command for bash, zsh, fish
- [ ] Dynamic task name completion

### Phase 5 Milestone

```bash
$ nix-tasks describe build
Task: build

Description:
  Build the application binary

Packages:
  - go
  - docker

Depended on by:
  - test
  - deploy

Inputs:
  - **/*.go
  - go.mod

$ nix-tasks
# Opens TUI for interactive exploration
```

---

## Phase 6: Standards & Migration

**Goal:** Enable company-wide distribution via flakes, implement Make compatibility layer.

### 6.1 Configuration Composition

```nix
# lib/compose.nix
{ lib }:
rec {
  # Override marker - completely replaces the value
  override = value: { __override = true; __value = value; };

  # Append to list
  append = values: { __append = true; __values = values; };

  # Prepend to list
  prepend = values: { __prepend = true; __values = values; };

  # Process overrides during merge
  processValue = base: overlay:
    if !builtins.isAttrs overlay then overlay
    else if overlay ? __override then overlay.__value
    else if overlay ? __append then base ++ overlay.__values
    else if overlay ? __prepend then overlay.__values ++ base
    else if builtins.isAttrs base then mergeAttrs base overlay
    else overlay;

  # Deep merge two attribute sets
  mergeAttrs = base: overlay:
    let
      baseKeys = builtins.attrNames base;
      overlayKeys = builtins.attrNames overlay;
      allKeys = lib.unique (baseKeys ++ overlayKeys);
    in
      builtins.listToAttrs (map (key:
        let
          baseVal = base.${key} or null;
          overlayVal = overlay.${key} or null;
          merged =
            if overlayVal == null then baseVal
            else if baseVal == null then overlayVal
            else processValue baseVal overlayVal;
        in
          { name = key; value = merged; }
      ) allKeys);

  # Extend a base configuration with overrides
  extend = base: overlay:
    let
      overlayResolved = if builtins.isFunction overlay then overlay base else overlay;
    in
      mergeAttrs base overlayResolved;
}
```

### 6.2 Make Compatibility

```nix
# lib/compat/make.nix
{ lib, pkgs }:
{
  # Wrap a single Make target
  mkMakeTask = {
    target,
    makefile ? "Makefile",
    description ? "Make target: ${target}",
    ...
  }@args:
    lib.mkTask ({
      inherit description;
      deps = ["gnumake"];
      commands = [
        "make -f ${makefile} ${target}"
      ];
    } // (builtins.removeAttrs args ["target" "makefile" "description"]));
}
```

### 6.3 Standards Template

```nix
# Example company-standards flake
{
  description = "Company-wide nix-tasks standards";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      lib = nix-tasks.lib;

      standardConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          packages = {
            go = pkgs.go_1_22;
            docker = pkgs.docker;
            kubectl = pkgs.kubectl;
            golangci-lint = pkgs.golangci-lint;
            gnumake = pkgs.gnumake;
          };

          tasks = {
            lint = lib.mkTask {
              description = "Run linters";
              deps = ["golangci-lint"];
              commands = ["golangci-lint run ./..."];
              continueOnError = true;
            };
          };

          devShells = {
            minimal = { packages = ["go"]; };
            ci = { extends = "minimal"; packages = ["docker" "golangci-lint"]; };
            default = { extends = "ci"; packages = ["kubectl"]; };
          };
        };
    in {
      lib = lib // {
        # Company-specific builders
        mkServiceTask = { ... }: /* ... */;
      };

      # Export standard config per system
      standardConfig = builtins.listToAttrs (map (system: {
        name = system;
        value = standardConfig system;
      }) ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"]);
    };
}
```

Usage in a repo:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    company-standards.url = "github:company/nix-tasks-standards";
  };

  outputs = { self, nixpkgs, company-standards }:
    let
      system = "x86_64-linux";
      lib = company-standards.lib;

      config = lib.evalConfig (lib.extend company-standards.standardConfig.${system} {
        # Add repo-specific tasks
        tasks.build = lib.mkGoTask {
          description = "Build the service";
          output = "bin/service";
        };

        tasks.deploy = lib.mkTask {
          description = "Deploy to Kubernetes";
          deps = ["kubectl"];
          depends = ["task:build"];
          commands = ["kubectl apply -f deploy/"];
        };

        # Override standard lint config
        tasks.lint.commands = lib.override ["golangci-lint run --timeout 5m ./..."];
      });
    in config // {
      devShells.${system} = config.devShells;
    };
}
```

#### Deliverables
- [ ] Implement `override`, `append`, `prepend` helpers
- [ ] Implement deep merge with override processing
- [ ] Implement `extend` function for composition
- [ ] Create Make compatibility helpers
- [ ] Create example company standards flake
- [ ] Write documentation for customization patterns

### Phase 6 Milestone

```bash
# Using company standards with customization
$ nix-tasks list
Tasks:
  build                Build the service
  test                 Run tests
  lint                 Run linters (standard)
  deploy               Deploy to Kubernetes
  make:legacy          Make target: legacy

$ nix-tasks run lint
✓ lint (using company standard config)
```

---

## Timeline Summary

| Phase | Duration Estimate | Dependencies |
|-------|------------------|--------------|
| Phase 1: Foundation | 2-3 weeks | None |
| Phase 2: Task Orchestration | 2-3 weeks | Phase 1 |
| Phase 3: Environment Management | 1-2 weeks | Phase 1 |
| Phase 4: Caching | 2 weeks | Phase 2 |
| Phase 5: Developer Experience | 2-3 weeks | Phases 2, 3 |
| Phase 6: Standards & Migration | 2 weeks | Phases 1-5 |

**Total estimated duration: 11-15 weeks**

Phases 2 and 3 can be worked on in parallel after Phase 1.

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Nix evaluation performance | Keep evaluations minimal; profile and optimize |
| Complex Nix learning curve | Builder library abstracts complexity |
| Cache invalidation bugs | Conservative fingerprinting; easy force-rebuild |
| Migration resistance | Make compatibility layer enables gradual adoption |

## Success Metrics

- [ ] Task execution matches or exceeds Make performance
- [ ] Configuration significantly more readable than Makefiles
- [ ] Dev shell activation under 5 seconds (with Nix cache)
- [ ] Successful pilot with 3+ repositories
- [ ] Clear migration path documented and tested
