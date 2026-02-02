# CLAUDE.md - Development Guidelines for nix-tasks

This document provides guidelines for Claude when implementing the nix-tasks project.

## Other Documents

- Implementation Plan - `./docs/implementation-plan.md`
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

```nix
{
  packages = {
    go = pkgs.go_1_22;
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
- `docs/implementation-plan.md` - Phased implementation plan with Go code examples

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
    "net/http"      // HTTP
    "encoding/json" // JSON
    "context"       // Context
    "sync"          // Concurrency primitives
)

// Only use external packages when necessary
import (
    "github.com/alecthomas/kong"           // CLI (no good stdlib alternative)
    tea "github.com/charmbracelet/bubbletea" // TUI (no stdlib alternative)
    "github.com/nix-community/go-nix/pkg/storepath" // Nix-specific
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
func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
    // Check context cancellation
    if ctx.Err() != nil {
        return ctx.Err()
    }

    // Pass context to subprocesses
    cmd := exec.CommandContext(ctx, "nix", args...)

    // Pass context to other functions
    result, err := e.cache.Lookup(ctx, name, fingerprint)
}
```

### Package Structure

Follow the standard Go project layout:

```
cmd/
    nix-tasks/
        main.go           # Entry point only - minimal code
internal/
    cli/                  # CLI command definitions (Kong)
        cli.go            # Root CLI struct
        run.go            # Each command in its own file
        list.go
    config/               # Configuration types and loading
        config.go         # Type definitions
        loader.go         # Loading logic
        validate.go       # Validation logic
    nix/                  # Nix integration
        evaluator.go      # Main Nix wrapper
        errors.go         # Nix-specific errors
    runner/               # Task execution
        executor.go       # Single task execution
        graph.go          # Dependency graph
        parallel.go       # Parallel execution
    cache/                # Caching layer
        store.go          # Cache storage
        fingerprint.go    # Content hashing
    ui/                   # Output formatting
        printer.go        # Status printing
        colors.go         # ANSI colors
    tui/                  # Interactive TUI
        app.go            # Bubbletea model
```

### Interface Design

Define interfaces where they're used, not where they're implemented:

```go
// In runner/executor.go - defines what it needs
type NixEvaluator interface {
    Eval(ctx context.Context, attr string, result any) error
    DevelopCmd(ctx context.Context, shell string, cmd []string) *exec.Cmd
}

// In nix/evaluator.go - implements the interface
type Evaluator struct {
    flakePath string
    debug     bool
}

// Satisfies NixEvaluator interface
func (e *Evaluator) Eval(ctx context.Context, attr string, result any) error {
    // implementation
}
```

### Testing Patterns

**Unit tests with mocks:**

```go
// internal/runner/executor_test.go
package runner_test

import (
    "context"
    "testing"

    "github.com/redbackthomson/nix-tasks/internal/runner"
)

// Mock for testing
type mockEvaluator struct {
    evalFunc func(ctx context.Context, attr string, result any) error
}

func (m *mockEvaluator) Eval(ctx context.Context, attr string, result any) error {
    if m.evalFunc != nil {
        return m.evalFunc(ctx, attr, result)
    }
    return nil
}

func TestExecutor_RunTask(t *testing.T) {
    mock := &mockEvaluator{
        evalFunc: func(ctx context.Context, attr string, result any) error {
            // Set up mock response
            return nil
        },
    }

    exec := runner.NewExecutor(mock, cfg, runner.ExecutorOptions{})
    err := exec.RunTask(context.Background(), "build", task)

    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
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

**Integration tests (require Nix):**

```go
//go:build integration

package integration_test

import (
    "context"
    "os/exec"
    "testing"
)

func TestNixTasksRun(t *testing.T) {
    // Skip if nix not available
    if _, err := exec.LookPath("nix"); err != nil {
        t.Skip("nix not available")
    }

    // Run actual nix-tasks command
    cmd := exec.Command("nix-tasks", "run", "build")
    cmd.Dir = "testdata/simple-project"

    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("nix-tasks run failed: %v\n%s", err, output)
    }
}
```

### Concurrency Patterns

**Goroutines with WaitGroup:**

```go
func (p *ParallelExecutor) executeGroup(ctx context.Context, tasks []string) []TaskResult {
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

            // Check cancellation
            if ctx.Err() != nil {
                results[idx] = TaskResult{Name: taskName, Error: ctx.Err()}
                return
            }

            // Execute
            err := p.executor.RunTask(ctx, taskName)
            results[idx] = TaskResult{Name: taskName, Success: err == nil, Error: err}
        }(i, name)
    }

    wg.Wait()
    return results
}
```

**Mutex for shared state:**

```go
type OutputManager struct {
    mu      sync.Mutex
    buffers map[string]*bytes.Buffer
}

func (m *OutputManager) Writer(taskName string) io.Writer {
    m.mu.Lock()
    defer m.mu.Unlock()

    buf := &bytes.Buffer{}
    m.buffers[taskName] = buf
    return buf
}
```

### CLI Patterns (Kong)

**Command structure:**

```go
// Root CLI with global flags
type CLI struct {
    Globals

    Run      RunCmd      `cmd:"" help:"Run a task"`
    List     ListCmd     `cmd:"" help:"List available tasks"`
    Shell    ShellCmd    `cmd:"" help:"Enter a development shell"`
    Cache    CacheCmd    `cmd:"" help:"Cache management commands"`
}

type Globals struct {
    Verbose bool   `short:"v" help:"Show task output"`
    Debug   bool   `help:"Show debug information"`
    Flake   string `short:"f" help:"Path to flake" default:"."`
}

// Command with arguments and flags
type RunCmd struct {
    Task            string `arg:"" help:"Task name to run"`
    Jobs            int    `short:"j" help:"Parallel jobs" default:"4"`
    Force           bool   `help:"Bypass cache"`
    ContinueOnError bool   `help:"Continue on task failure"`
}

func (c *RunCmd) Run(globals *Globals) error {
    // Implementation
}
```

### Output Formatting

**Colors (no icons):**

```go
const (
    colorReset = "\033[0m"
    colorRed   = "\033[31m"
    colorGreen = "\033[32m"
)

func Green(s string) string {
    return fmt.Sprintf("%s%s%s", colorGreen, s, colorReset)
}

func Red(s string) string {
    return fmt.Sprintf("%s%s%s", colorRed, s, colorReset)
}

// Usage
fmt.Printf("%s %s\n", Green("✓"), taskName)  // Success
fmt.Printf("%s %s\n", Red("✗"), taskName)    // Failure
```

**Detect terminal vs CI:**

```go
func isCI() bool {
    return os.Getenv("CI") != ""
}

func isTerminal() bool {
    fileInfo, _ := os.Stdout.Stat()
    return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
```

## Nix Integration Patterns

### Subprocess Wrapper

```go
type Evaluator struct {
    flakePath string
    debug     bool
}

func (e *Evaluator) Eval(ctx context.Context, attr string, result any) error {
    expr := fmt.Sprintf("%s#%s", e.flakePath, attr)
    args := []string{"eval", "--json", expr}

    if e.debug {
        slog.Debug("nix eval", "expr", expr)
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

    return json.Unmarshal(stdout.Bytes(), result)
}
```

### Running Commands in Nix Develop

```go
func (e *Evaluator) DevelopCmd(ctx context.Context, shell string, command []string) *exec.Cmd {
    expr := fmt.Sprintf("%s#%s", e.flakePath, shell)
    args := []string{"develop", expr, "--command"}
    args = append(args, command...)

    return exec.CommandContext(ctx, "nix", args...)
}

// Usage
cmd := eval.DevelopCmd(ctx, "nixTasksShells.build", []string{"bash", "-e", "-c", script})
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
err := cmd.Run()
```

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
