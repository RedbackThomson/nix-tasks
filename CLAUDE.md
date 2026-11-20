# CLAUDE.md - Development Guidelines for nix-tasks

This document provides guidelines for Claude when implementing the nix-tasks project.

## Other Documents

- Requirements - `./docs/requirements.md`

## Project Summary

### What We're Building

**nix-tasks** is a Nix-based task runner and development environment manager that replaces Makefiles across an organization. Think of it as `just` or `mise` but with Nix's power for reproducibility, composition, and environment management.

### The Problem

- Makefiles become complex and unreadable with templates and shell escaping
- Each repository manages its own build tooling independently
- No standard way to share and customize tooling across repositories
- Development environments are inconsistent between machines and CI

### The Solution

A unified system that provides:

1. **Readable task definitions** - Declarative Nix configuration with helper builders (`mkGoTask`, `mkDockerTask`, etc.) instead of cryptic Make syntax

2. **Company-wide standards** - A base flake that repositories import and extend, ensuring consistency while allowing customization

3. **Composable configuration** - Deep merge with explicit override markers (`override`, `append`, `prepend`) so repos can customize standards without forking

4. **Reproducible environments** - Dev shells with inheritance (minimal → ci → default) ensuring tasks run with exact package versions

5. **Version consistency** - Shared package registry ensures tasks and dev shells use identical tool versions

6. **Smart caching** - Hybrid Nix store + task-level fingerprinting for fast incremental builds

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Core architecture | Unified config layer (tasks & environments are equal) |
| Composition model | Deep merge with override markers |
| Task execution | Hybrid declarative → shell scripts |
| Dependencies | Explicit DAG with parallel execution |
| Environment strategy | Shell inheritance/composition |
| Caching | Nix store + task fingerprinting |
| Distribution | Flake inputs with version pinning |
| Migration | Gradual with Make compatibility layer |

### User Experience Goals

```bash
# List available tasks
$ nix-tasks list
Tasks:
  build       Build the application
  test        Run tests
  deploy      Deploy to Kubernetes

# Run a task (with dependencies)
$ nix-tasks run deploy
✓ build (2.3s)
✓ test (4.1s)
✓ deploy (0.8s)

# Enter development shell
$ nix-tasks shell
(dev) $ go build ./...   # Tools available at pinned versions

# Interactive TUI for exploration
$ nix-tasks
# Opens interactive menu
```

### Example Configuration

The attribute set passed to `lib.evalConfig`:

```nix
{
  packages = {
    go = pkgs.go_1_22;
    docker = pkgs.docker;
  };

  tasks = {
    build = lib.mkGoTask {
      description = "Build the application";
      pname = "app";
      src = ./.;
      vendorHash = null;
    };

    test = lib.mkTask {
      description = "Run tests";
      deps = ["go"];
      depends = ["task:build"];
      commands = ["go test ./..."];
    };
  };

  devShells = {
    ci = { packages = ["go" "docker"]; };
    default = {
      extends = "ci";
      packages = ["kubectl" "k9s"];
    };
  };
}
```

## Key Documents

- `docs/requirements.md` - Full system requirements and design decisions

## Source Control: jj (Jujutsu)

This project uses **jj (Jujutsu)** instead of git. Use these commands:

```bash
# Status and log
jj status                    # Show working copy status
jj log                       # Show commit history
jj diff                      # Show changes in working copy

# Creating commits
jj new                       # Create a new empty commit on top of current
jj commit -m "message"       # Commit current changes with message
jj describe -m "message"     # Change the description of the current commit

# Branches and navigation
jj branch create <name>      # Create a branch at current commit
jj branch set <name>         # Move branch to current commit
jj new <revision>            # Create new commit on top of revision
jj edit <revision>           # Edit an existing commit

# Rebasing and editing history
jj rebase -d <destination>   # Rebase current commit onto destination
jj squash                    # Squash current commit into parent
jj split                     # Split current commit interactively

# Working with remotes
jj git fetch                 # Fetch from git remote
jj git push                  # Push to git remote

# Resolving conflicts
jj resolve                   # Launch merge tool for conflicts
jj restore                   # Restore files from parent commit
```

**Commit message format:**
```
<type>: <short description>

<optional body>
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

## Go Conventions

### Standard Library First

Always prefer standard library packages over third-party alternatives:

```go
// Preferred
import (
    "log/slog"      // Logging
    "errors"        // Error handling
    "testing"       // Testing
    "encoding/json" // JSON
    "context"       // Context
    "sync"          // Concurrency primitives
)

// The external packages in use, each with no stdlib alternative
import (
    "github.com/alecthomas/kong"                     // CLI
    tea "github.com/charmbracelet/bubbletea"         // TUI
    "github.com/charmbracelet/lipgloss"              // TUI styling
    "github.com/charmbracelet/x/term"                // terminal size
    "github.com/bmatcuk/doublestar/v4"               // ** globs for task inputs
    "github.com/nix-community/go-nix/pkg/storepath"  // Nix store paths
)
```

### Error Handling

Use Go 1.20+ error wrapping patterns:

```go
// Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}

// Custom error types for specific cases
type ValidationError struct {
    Errors []string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation failed:\n  %s", strings.Join(e.Errors, "\n  "))
}

// Check error types
var validErr *ValidationError
if errors.As(err, &validErr) {
    // Handle validation error
}

// Check sentinel errors
if errors.Is(err, context.Canceled) {
    // Handle cancellation
}
```

### Structured Logging

Use `log/slog` for all logging:

```go
import "log/slog"

// Debug logging (only shown with --debug flag)
slog.Debug("running nix command", "cmd", "nix", "args", args)

// Info logging
slog.Info("task completed", "name", taskName, "duration", duration)

// Error logging
slog.Error("task failed", "name", taskName, "error", err)

// Create a logger with context
logger := slog.With("task", taskName)
logger.Info("starting execution")
```

### Context Usage

Always pass context as the first parameter:

```go
func buildAttr(ctx context.Context, flake, attr string) error {
    // Check context cancellation
    if err := ctx.Err(); err != nil {
        return err
    }

    // Pass context to subprocesses so cancellation stops them
    cmd := exec.CommandContext(ctx, "nix", "build", flake+"#"+attr)
    return cmd.Run()
}
```

### Package Structure

Follow the standard Go project layout:

```
cmd/
    nix-tasks/
        main.go           # Entry point only - minimal code
internal/
    cli/                  # Kong commands, one file per command (run.go, list.go, ...)
    config/               # Types (config.go), loading (loader.go), validation (validate.go)
    nix/                  # nix CLI wrapper (evaluator.go) and its errors (errors.go)
    runner/               # Dependency graph (graph.go) and task execution
    cache/                # Fingerprints (fingerprint.go) and the on-disk store (store.go)
    ui/                   # Colours, status printing and the progress display
    tui/                  # Bubbletea app (app.go)
lib/                      # Nix library: evalConfig, builders, shells, composition, Make compat
```

### Interface Design

Define interfaces where they're used, not where they're implemented. The
consumer declares only the methods it calls:

```go
type evaluator interface {
    Eval(ctx context.Context, attr string, result any) error
}

// *nix.Evaluator satisfies evaluator without referring to it.
func loadTasks(ctx context.Context, eval evaluator) (map[string]config.Task, error) {
    var tasks map[string]config.Task
    err := eval.Eval(ctx, "nixTasksConfig."+nix.CurrentSystem()+".tasks", &tasks)
    return tasks, err
}
```

### Testing Patterns

**Unit tests with fakes:** code that takes an interface can be tested without Nix.

```go
type fakeEvaluator struct {
    values map[string]string // attribute -> JSON
}

func (f *fakeEvaluator) Eval(_ context.Context, attr string, result any) error {
    v, ok := f.values[attr]
    if !ok {
        return fmt.Errorf("attribute %s not found", attr)
    }
    return json.Unmarshal([]byte(v), result)
}
```

**Table-driven tests:**

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        config  *config.Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: &config.Config{
                Packages: map[string]string{"go": "/nix/store/..."},
                Tasks:    map[string]config.Task{"build": {Deps: []string{"go"}}},
            },
            wantErr: false,
        },
        {
            name: "unknown package in task",
            config: &config.Config{
                Packages: map[string]string{},
                Tasks:    map[string]config.Task{"build": {Deps: []string{"go"}}},
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := config.Validate(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Integration tests (require Nix):** they live in `tests/integration`, carry
the `integration` build tag, run the `nix-tasks` binary against the flakes in
`tests/integration/testdata`, and skip when `nix` is not on `PATH`.

```go
//go:build integration

package integration_test
```

### Concurrency Patterns

**Goroutines with a semaphore and WaitGroup:**

```go
func runAll(ctx context.Context, items []string, jobs int, work func(context.Context, string) error) []error {
    errs := make([]error, len(items))
    sem := make(chan struct{}, jobs)
    var wg sync.WaitGroup

    for i, item := range items {
        wg.Add(1)
        go func() {
            defer wg.Done()

            sem <- struct{}{}
            defer func() { <-sem }()

            if err := ctx.Err(); err != nil {
                errs[i] = err
                return
            }
            errs[i] = work(ctx, item)
        }()
    }

    wg.Wait()
    return errs
}
```

**Mutex for shared state:**

```go
type counter struct {
    mu     sync.Mutex
    counts map[string]int
}

func (c *counter) add(name string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.counts[name]++
}
```

### CLI Patterns (Kong)

**Command structure** (`internal/cli/cli.go`):

```go
// Globals contains flags available to all commands
type Globals struct {
	Verbose bool   `short:"v" help:"Show task output"`
	Debug   bool   `help:"Show debug information including Nix commands"`
	Flake   string `short:"f" help:"Path to flake" default:"."`

	// FlakeRoot is the local directory containing flake.nix, or empty when
	// Flake is not a local path. Set by ResolveFlake.
	FlakeRoot string `kong:"-"`
}

// CLI is the root command structure
type CLI struct {
	Globals

	Run         RunCmd         `cmd:"" help:"Run a task"`
	List        ListCmd        `cmd:"" help:"List available tasks and shells"`
	Describe    DescribeCmd    `cmd:"" help:"Show task details"`
	Graph       GraphCmd       `cmd:"" help:"Show execution graph for a task"`
	Shell       ShellCmd       `cmd:"" help:"Enter a development shell"`
	Cache       CacheCmd       `cmd:"" help:"Cache management commands"`
	Validate    ValidateCmd    `cmd:"" help:"Validate configuration"`
	TUI         TUICmd         `cmd:"" default:"withargs" help:"Launch interactive TUI (default)"`
	Completions CompletionsCmd `cmd:"" help:"Generate shell completions"`
}
```

**Command with arguments and flags** (abridged from `internal/cli/run.go`):

```go
type RunCmd struct {
	Task            string `arg:"" help:"Task name to run"`
	Jobs            int    `short:"j" help:"Number of parallel jobs" default:"4"`
	ContinueOnError bool   `help:"Continue running independent tasks on failure"`
}

func (c *RunCmd) Run(globals *Globals) error {
	// Implementation
}
```

### Output Formatting

**Colors (no icons):** `internal/ui/colors.go` wraps strings in ANSI colours
with `Green`, `Red`, `Yellow`, `Blue` and `Gray`:

```go
fmt.Printf("%s %s\n", ui.Green("✓"), taskName)  // Success
fmt.Printf("%s %s\n", ui.Red("✗"), taskName)    // Failure
```

**Terminal vs CI:** `runner.DetectOutputMode` streams prefixed output when `CI`
is set, shows the live progress display when stdout is a terminal, and
otherwise buffers each task's output and prints it only on failure.

## Nix Integration Patterns

### Subprocess Wrapper

`internal/nix/evaluator.go` runs the `nix` CLI as a subprocess. `Eval` runs
`nix eval --json <flake>#<attr>`, logs the command with `slog.Debug`, and
returns an `*EvalError` carrying the attribute and nix's stderr on failure
(simplified):

```go
func (e *Evaluator) Eval(ctx context.Context, attr string, result any) error {
	expr := fmt.Sprintf("%s#%s", e.flakePath, attr)
	cmd := exec.CommandContext(ctx, "nix", "eval", "--json", expr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &EvalError{Attribute: attr, Stderr: stderr.String(), Err: err}
	}
	return json.Unmarshal(stdout.Bytes(), result)
}
```

### Running Task Commands

Shell tasks run their script inside the task's shell:
`Evaluator.DevelopCmd` builds `nix develop <flake>#nixTasksShells."<system>"."<task>"
--command bash -e -c <script>`, where the script is the task's `script` or its
`commands` joined by newlines. Build tasks run
`nix build <flake>#nixTasksBuilds."<system>"."<task>"^* --no-link --json`
(`Evaluator.BuildCmd`) and link each output nix reports to
`.nix-tasks/<task>/<output>`.

Build flake attribute paths with the helpers in `internal/nix/attrs.go`
(`AttrPath`, `NixTasksShellsAttr`, `NixTasksBuildsAttr`, `DevShellAttr`),
which quote each name so a task such as `lint.go` stays one attribute.

## Code Style

### Formatting

Always run before committing:

```bash
go fmt ./...
goimports -w .
```

### Linting

Use golangci-lint with these enabled linters:

```yaml
# .golangci.yml
linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
```

Run with:

```bash
golangci-lint run ./...
```

### Naming Conventions

```go
// Packages: lowercase, single word
package runner
package config

// Exported types: PascalCase
type TaskExecutor struct {}
type Config struct {}

// Unexported types: camelCase
type taskResult struct {}

// Interfaces: -er suffix when appropriate
type Evaluator interface {}
type Runner interface {}

// Constants: PascalCase for exported, camelCase for unexported
const MaxParallelJobs = 8
const defaultTimeout = 30 * time.Second

// Errors: Err prefix
var ErrTaskNotFound = errors.New("task not found")
var ErrCircularDependency = errors.New("circular dependency detected")
```

## File Templates

### New Command File

```go
// internal/cli/newcmd.go
package cli

import (
    "context"
    "fmt"

    "github.com/redbackthomson/nix-tasks/internal/config"
    "github.com/redbackthomson/nix-tasks/internal/nix"
)

type NewCmd struct {
    // Arguments and flags
}

func (c *NewCmd) Run(globals *Globals) error {
    ctx := context.Background()

    eval := nix.NewEvaluator(globals.Flake)
    eval.SetDebug(globals.Debug)

    cfg, err := config.Load(ctx, eval)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Implementation

    return nil
}
```

### New Test File

```go
// internal/pkg/thing_test.go
package pkg_test

import (
    "testing"

    "github.com/redbackthomson/nix-tasks/internal/pkg"
)

func TestThing(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        // Test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := pkg.Thing(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Thing() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Thing() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Common Tasks

### Adding a New CLI Command

1. Create `internal/cli/cmdname.go` with command struct and `Run` method
2. Add command to `CLI` struct in `internal/cli/cli.go`
3. Write tests in `internal/cli/cmdname_test.go`

### Adding a New Nix Builder

1. Add builder function to `lib/builders.nix`
2. Export from `lib/default.nix`
3. Document usage in examples

### Running Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Integration tests (requires Nix)
go test -tags=integration ./tests/integration/...

# Specific package
go test ./internal/runner/...
```

### Building

```bash
# Development build
go build -o nix-tasks ./cmd/nix-tasks

# With Nix (reproducible)
nix build

# Run directly with Nix
nix run . -- list
```
