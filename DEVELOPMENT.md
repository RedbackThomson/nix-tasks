# Development Guide

Guide for building, testing, and contributing to nix-tasks.

## Prerequisites

- Nix with flakes enabled

## Getting Started

Enter the development shell:

```bash
nix develop
```

This provides Go and golangci-lint at pinned versions.

## Building

### Development Build

```bash
go build -o nix-tasks ./cmd/nix-tasks
```

### Nix Build (Reproducible)

```bash
nix build
./result/bin/nix-tasks --help
```

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Test specific package
go test ./internal/runner/...
```

### Integration Tests

Integration tests require Nix and are tagged with `//go:build integration`:

```bash
# Run integration tests
go test -tags=integration ./tests/integration/...

# Run specific test
go test -tags=integration ./tests/integration -run TestRun_SimpleTask
```

Integration tests build the nix-tasks binary and test against real Nix flakes in `tests/integration/testdata/`.

## Code Style

### Formatting and Linting

```bash
# Format code
go fmt ./...

# Run linter
golangci-lint run ./...
```

Always format and lint before committing.

## Code Conventions

### Standard Library First

Prefer standard library over third-party packages:

```go
import (
    "log/slog"      // Logging
    "errors"        // Error handling
    "context"       // Context
    "net/http"      // HTTP
)
```

Only use external packages when necessary (Kong for CLI, Bubbletea for TUI, go-nix for Nix integration).

### Error Handling

Wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}
```

Check errors with `errors.Is` and `errors.As`:

```go
if errors.Is(err, context.Canceled) {
    return nil
}
```

### Structured Logging

Use `log/slog` for all logging:

```go
slog.Debug("running command", "cmd", "nix", "args", args)
slog.Info("task completed", "name", taskName, "duration", duration)
slog.Error("task failed", "name", taskName, "error", err)
```

### Context Passing

Always pass context as the first parameter:

```go
func (e *Executor) RunTask(ctx context.Context, name string, task config.Task) error {
    if ctx.Err() != nil {
        return ctx.Err()
    }
    // ...
}
```

### Testing Patterns

Use table-driven tests:

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {name: "valid", input: "foo", wantErr: false},
        {name: "invalid", input: "", wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("got error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Interface Design

Define interfaces where they're used, not where they're implemented:

```go
// In runner/executor.go
type NixEvaluator interface {
    Eval(ctx context.Context, attr string, result any) error
}

// In nix/evaluator.go
type Evaluator struct { /* ... */ }

func (e *Evaluator) Eval(ctx context.Context, attr string, result any) error {
    // implementation
}
```

## Project Structure

```
cmd/nix-tasks/          # Entry point
internal/
  cli/                  # CLI commands (Kong)
  config/               # Configuration loading/validation
  nix/                  # Nix subprocess wrapper
  runner/               # Task execution
  cache/                # Caching layer
  ui/                   # Output formatting
  tui/                  # Interactive TUI
lib/                    # Nix library (builders, evaluators)
tests/integration/      # Integration tests
```

## Additional Resources

- [CLAUDE.md](CLAUDE.md) - Detailed development guidelines
- [docs/requirements.md](docs/requirements.md) - System requirements
- [examples/](examples/) - Example projects

## License

MIT
