# nix-tasks Demo

This example demonstrates nix-tasks with a mock Go web service project.

## Task Dependency Graph

```
                    ┌─────────┐
                    │ generate│
                    └────┬────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
         ▼               ▼               ▼
    ┌────────┐      ┌────────┐      ┌──────────┐
    │  lint  │      │ build  │      │test-unit │
    └────────┘      └───┬────┘      └──────────┘
                        │
    ┌───────────────┐   │
    │ build-frontend│   │
    └───────┬───────┘   │
            │           ▼
            │    ┌────────────────┐
            │    │test-integration│
            │    └───────┬────────┘
            │            │
            │      ┌─────┴─────┐
            │      │   test    │
            │      └─────┬─────┘
            │            │
            └──────┬─────┘
                   ▼
           ┌──────────────┐
           │ docker-build │
           └──────┬───────┘
                  │
                  ▼
           ┌──────────────┐
           │ docker-push  │
           └──────┬───────┘
                  │
                  ▼
          ┌───────────────┐
          │ deploy-staging│
          └───────┬───────┘
                  │
                  ▼
           ┌─────────────┐
           │ deploy-prod │
           └─────────────┘
```

## Quick Start

### Option 1: Use in development shell (recommended)

```bash
# Enter the development shell (includes nix-tasks)
nix develop

# List all available tasks
nix-tasks list

# Run a simple task
nix-tasks run build

# Run the full CI pipeline (runs lint, tests, docker build)
nix-tasks run ci

# See task details and dependencies
nix-tasks describe deploy-staging

# Validate configuration
nix-tasks validate

# Clean up
nix-tasks run clean
```

### Option 2: Run tasks directly without entering shell

```bash
# Run tasks directly with nix run
nix run .#nix-tasks -- list
nix run .#nix-tasks -- run build
nix run .#nix-tasks -- run ci

# Or use the default app (shorter)
nix run . -- list
nix run . -- run build
nix run . -- run ci
```

## Tasks

| Task | Description | Dependencies |
|------|-------------|--------------|
| `generate` | Generate mocks and code | - |
| `build` | Build the application | generate |
| `build-frontend` | Build frontend assets | - |
| `lint` | Run linters | generate |
| `fmt` | Format code | - |
| `test-unit` | Run unit tests | generate |
| `test-integration` | Run integration tests | build |
| `test` | Run all tests | test-unit, test-integration |
| `docker-build` | Build Docker image | build, build-frontend |
| `docker-push` | Push Docker image | docker-build |
| `deploy-staging` | Deploy to staging | docker-push, test |
| `deploy-prod` | Deploy to production | deploy-staging |
| `clean` | Clean build artifacts | - |
| `health-check` | Check service health | - |
| `ci` | Full CI pipeline | lint, test, docker-build |
| `release` | Full release pipeline | ci, docker-push, deploy-staging |

## Dev Shells

Three shells with inheritance:

```
minimal (go)
    │
    ▼
   ci (+ jq)
    │
    ▼
default (+ nodejs, curl)
```

```bash
# Enter minimal shell (just Go)
nix develop .#minimal

# Enter CI shell (Go + jq)
nix develop .#ci

# Enter full dev shell (everything)
nix develop
```
