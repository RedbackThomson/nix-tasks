# nix-tasks

A Nix-based task runner that brings reproducibility to your build scripts.

Define tasks in Nix, run them with exact package versions, and share configurations across your organization.

## Why nix-tasks?

**The problem:** Makefiles become unreadable. Shell scripts break on different machines. Your CI uses different tool versions than your laptop.

**The solution:** Define tasks declaratively in Nix. Every task runs with pinned dependencies. Share task definitions across repositories.

```nix
tasks = {
  build = lib.mkTask {
    description = "Build the application";
    deps = [ "go" ];  # Uses exactly the Go version defined in packages
    commands = [ "go build -o bin/app ./..." ];
  };
};
```

## Comparison

| Feature | Make | just | nix-tasks |
|---------|------|------|-----------|
| Readable syntax | No | Yes | Yes |
| Reproducible environments | No | No | **Yes** |
| Pinned tool versions | No | No | **Yes** |
| Shareable across repos | No | No | **Yes** |
| Dev shell integration | No | No | **Yes** |

nix-tasks is for teams who want the reproducibility of Nix with the simplicity of a task runner.

## Installation

### Prerequisites

- [Nix](https://nixos.org/download.html) with flakes enabled

### Run directly

```bash
nix run github:redbackthomson/nix-tasks -- --help
```

### Add to your project

In your `flake.nix`:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    # ... see Quick Start below
}
```

## Quick Start

### 1. Create a flake.nix

```nix
{
  description = "My project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      system = "aarch64-darwin";  # or x86_64-linux, etc.
      pkgs = nixpkgs.legacyPackages.${system};
      lib = nix-tasks.lib.${system};

      config = lib.evalConfig {
        # Define available packages
        packages = {
          go = pkgs.go;
          nodejs = pkgs.nodejs;
        };

        # Define tasks
        tasks = {
          build = lib.mkTask {
            description = "Build the application";
            deps = [ "go" ];
            commands = [
              "go build -o bin/app ./cmd/app"
            ];
          };

          test = lib.mkTask {
            description = "Run tests";
            deps = [ "go" ];
            commands = [ "go test ./..." ];
          };

          clean = lib.mkTask {
            description = "Clean build artifacts";
            commands = [ "rm -rf bin/" ];
          };
        };

        # Define dev shells
        devShells = {
          default = {
            packages = [ "go" "nodejs" ];
            shellHook = ''
              echo "Ready to develop!"
            '';
          };
        };
      };
    in {
      # Expose nix-tasks config
      inherit (config) nixTasksConfig nixTasksShells;

      # Expose nix-tasks CLI as a package and app (optional but recommended)
      packages.${system} = {
        nix-tasks = nix-tasks.packages.${system}.default;
        default = nix-tasks.packages.${system}.default;
      };

      apps.${system} = {
        nix-tasks = nix-tasks.apps.${system}.default;
        default = nix-tasks.apps.${system}.default;
      };

      # Expose dev shells with nix-tasks available
      devShells.${system}.default = config.devShells.default.overrideAttrs (old: {
        buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
      });
    };
}
```

### 2. List available tasks

You can run nix-tasks in three ways:

#### Option A: From the global nix-tasks (requires --flake flag)

```bash
$ nix run github:redbackthomson/nix-tasks -- list --flake .

Tasks:
  build                Build the application
  clean                Clean build artifacts
  test                 Run tests
```

#### Option B: Using your project's exposed app (recommended)

```bash
# Run via the nix-tasks package you exposed
$ nix run .#nix-tasks -- list

# Or use the default app (shorter)
$ nix run . -- list

Tasks:
  build                Build the application
  clean                Clean build artifacts
  test                 Run tests
```

#### Option C: In a development shell

```bash
# Enter the dev shell (which includes nix-tasks)
$ nix develop

# Now run nix-tasks directly
$ nix-tasks list
Tasks:
  build                Build the application
  clean                Clean build artifacts
  test                 Run tests
```

### 3. Run a task

```bash
# Using your project's exposed app
$ nix run . -- run build
✓ build

# Or in the dev shell
$ nix develop
$ nix-tasks run build
✓ build
```

With verbose output:

```bash
$ nix run . -- run build -v

go build -o bin/app ./cmd/app
✓ build
```

### 4. Describe a task

```bash
$ nix run github:redbackthomson/nix-tasks -- describe build

Task: build

Description:
  Build the application

Packages:
  - go

Depended on by:
  - deploy
```

### 5. Validate configuration

```bash
$ nix run github:redbackthomson/nix-tasks -- validate

✓ Configuration is valid
  Tasks: 3
  Packages: 2
  Dev Shells: 1
```

## Usage

### Commands

```
nix-tasks <command> [flags]

Commands:
  run <task>       Run a task (with dependencies)
  list             List available tasks and shells
  describe <task>  Show task details
  shell [name]     Enter a development shell
  cache clean      Clear the task cache
  cache stats      Show cache statistics
  validate         Validate configuration
  tui              Launch interactive TUI (default)
  completions      Generate shell completions

Global Flags:
  -v, --verbose    Show task output
  --debug          Show debug information
  -f, --flake      Path to flake (default: ".")
```

### Interactive TUI

When you run `nix-tasks` without arguments, it launches an interactive terminal UI:

```bash
$ nix-tasks

nix-tasks

Tasks
> build - Build the application
  test - Run tests
  deploy - Deploy to staging

Dev Shells
  default
  ci

[j/k] navigate  [tab] switch section  [enter/r] run  [q] quit
```

Navigate with `j/k` or arrow keys, `tab` to switch between tasks and shells, and `enter` to run.

### Running Tasks

```bash
# Run a single task
$ nix-tasks run build

# Run with dependencies (automatically runs task:build first)
$ nix-tasks run deploy
✓ build (2.1s)
✓ deploy (0.8s)

# Parallel execution with 8 jobs
$ nix-tasks run ci -j 8

# Continue running independent tasks even if one fails
$ nix-tasks run ci --continue-on-error

# Force rebuild (bypass cache)
$ nix-tasks run build --force

# Disable caching entirely
$ nix-tasks run build --no-cache

# Stream output in real-time (default in CI)
$ nix-tasks run build --stream
```

### Development Shells

```bash
# Enter the default shell
$ nix-tasks shell

# Enter a specific shell
$ nix-tasks shell ci

# Run a command in a shell without entering it
$ nix-tasks shell ci --command "go version"
```

### Caching

nix-tasks caches task results based on:
- Task commands and configuration
- Package store paths (Nix handles this)
- Input file contents (when `inputs` is specified)

```bash
# First run executes the task
$ nix-tasks run build
✓ build (3.2s)

# Second run uses cache
$ nix-tasks run build
✓ build (cached)

# Force rebuild
$ nix-tasks run build --force
✓ build (3.1s)

# Clear cache for this project
$ nix-tasks cache clean
Cache cleared

# View cache statistics
$ nix-tasks cache stats
Cache Statistics:
  Location: /Users/you/.cache/nix-tasks/abc123
  Entries: 5
  Size: 2.1 KB
```

### Shell Completions

Generate completions for your shell:

```bash
# Bash
$ nix-tasks completions bash >> ~/.bashrc

# Zsh
$ nix-tasks completions zsh >> ~/.zshrc

# Fish
$ nix-tasks completions fish > ~/.config/fish/completions/nix-tasks.fish
```

### Defining Tasks

#### Basic task

```nix
tasks.build = lib.mkTask {
  description = "Build the app";
  deps = [ "go" ];           # Package dependencies
  commands = [
    "go build -o bin/app ./..."
  ];
};
```

#### Task with environment variables

```nix
tasks.deploy = lib.mkTask {
  description = "Deploy to staging";
  deps = [ "kubectl" ];
  commands = [ "kubectl apply -f deploy/" ];
  env = {
    KUBECONFIG = "/path/to/config";
    NAMESPACE = "staging";
  };
};
```

#### Task with dependencies on other tasks

```nix
tasks.deploy = lib.mkTask {
  description = "Deploy (after build and test)";
  depends = [ "task:build" "task:test" ];
  commands = [ "kubectl apply -f deploy/" ];
};
```

#### Task with inputs/outputs (for caching)

```nix
tasks.build = lib.mkTask {
  description = "Build the app";
  deps = [ "go" ];
  commands = [ "go build -o bin/app ./..." ];
  inputs = [ "**/*.go" "go.mod" "go.sum" ];
  outputs = [ "bin/app" ];
};
```

### Task Builders

nix-tasks provides helper builders for common task types:

#### mkGoTask

```nix
tasks.build = lib.mkGoTask {
  description = "Build Go application";
  output = "bin/myapp";
  # Automatically adds "go" to deps and generates go build command
};
```

#### mkDockerTask

```nix
tasks.docker = lib.mkDockerTask {
  description = "Build Docker image";
  image = "myapp";
  tag = "latest";
  # Automatically adds "docker" to deps
};
```

#### mkCompoundTask

```nix
tasks.ci = lib.mkCompoundTask {
  description = "Run full CI pipeline";
  tasks = [ "lint" "test" "build" ];
  # Creates a task that depends on all listed tasks
};
```

### Task Modifiers

Task modifiers let you customize existing tasks without redefining them from
scratch. This is especially useful when extending standardized configurations in
a repository.

#### Modifying task dependencies

```nix
# Prepend a step before the existing dependencies
tasks.publish = lib.prependTaskDeps [ "generate-bindings" ] standards.tasks.publish;

# Append a step after
tasks.publish = lib.appendTaskDeps [ "notify" ] standards.tasks.publish;
```

#### Modifying commands

```nix
# Prepend a setup step to existing commands
tasks.test = lib.prependCommands
  [ "echo 'Running setup...'" ]
  standards.tasks.test;

# Override commands entirely
tasks.build = lib.overrideCommands
  [ "go build -o bin/myservice ./cmd/myservice" ]
  standards.tasks.build;
```

#### Chaining modifications with pipe

Apply multiple modifications at once using `lib.pipe`:

```nix
tasks.build = lib.pipe standards.tasks.build [
  (lib.overrideCommands [ "go build -o bin/myservice ./cmd/myservice" ])
  (lib.appendDeps [ "protobuf" ])
  (lib.mergeEnv { BUILD_VERSION = "1.0.0"; })
  (lib.setDescription "Build myservice")
];
```

#### Available modifiers

| Modifier | Description |
|----------|-------------|
| `prependTaskDeps` / `appendTaskDeps` / `overrideTaskDeps` | Modify task dependencies |
| `prependDeps` / `appendDeps` / `overrideDeps` | Modify package dependencies |
| `prependCommands` / `appendCommands` / `overrideCommands` | Modify shell commands |
| `mergeEnv` / `overrideEnv` | Modify environment variables |
| `appendInputs` / `overrideInputs` | Modify input file patterns |
| `setDescription` / `setWorkingDir` / `setNoCache` / `setContinueOnError` | Set task metadata |
| `pipe` | Apply a list of modifiers sequentially |

All modifiers are curried (`modification -> task -> task`) so they work with partial application and `pipe`.

### Dev Shells

Define development environments with shell inheritance:

```nix
devShells = {
  # Minimal shell
  minimal = {
    packages = [ "go" ];
  };

  # CI shell extends minimal
  ci = {
    extends = "minimal";
    packages = [ "docker" "golangci-lint" ];
    env = { CI = "true"; };
  };

  # Full dev shell extends CI
  default = {
    extends = "ci";
    packages = [ "kubectl" "k9s" ];
    shellHook = ''
      echo "Welcome to the dev shell!"
    '';
  };
};
```

Enter a shell:

```bash
# Default shell (using nix-tasks)
$ nix-tasks shell

# Specific shell
$ nix-tasks shell ci

# Or use nix develop directly
$ nix develop
$ nix develop .#ci
```

## Multi-System Support

Support multiple architectures:

```nix
{
  outputs = { self, nixpkgs, nix-tasks }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
        in lib.evalConfig {
          packages = { go = pkgs.go; };
          tasks = { /* ... */ };
          devShells = { /* ... */ };
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
```

## Examples

See the [examples](./examples) directory:

- **[simple](./examples/simple)** - Minimal example with build, test, clean tasks
- **[demo](./examples/demo)** - Full example with 16 tasks, dependencies, and multiple dev shells
- **[company-standards](./examples/company-standards)** - Company-wide standards flake template
- **[app-using-standards](./examples/app-using-standards)** - Repository extending company standards

## Roadmap

- [x] **Phase 1:** Core task runner (list, run, describe, validate)
- [x] **Phase 2:** Task dependencies with parallel execution
- [x] **Phase 3:** Dev shell inheritance
- [x] **Phase 4:** Task caching
- [x] **Phase 5:** Interactive TUI and shell completions
- [ ] **Phase 6:** Organization-wide task sharing

## Development

```bash
# Enter dev shell
nix develop

# Build
go build -o nix-tasks ./cmd/nix-tasks

# Test
go test ./...

# Lint
golangci-lint run ./...
```

## License

MIT
