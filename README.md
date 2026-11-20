# nix-tasks

A Nix-based task runner that brings reproducibility to your build scripts.

Define tasks in Nix, run them with exact package versions, and share configurations across your organization.

## Why nix-tasks?

**The problem:** Makefiles become unreadable. Shell scripts break on different machines. Your CI uses different tool versions than your laptop.

**The solution:** Define tasks declaratively in Nix. Every task runs with pinned dependencies. Share task definitions across repositories.

```nix
tasks.build = {
  description = "Build the application";
  deps = [ "go" ];  # Uses exactly the Go version defined in packages
  commands = [ "go build -o bin/app ./..." ];
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
    # Build tasks and shells with your nixpkgs, so task tools match the
    # versions you declare in `packages`.
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [ ./nix-tasks.nix ];  # see Quick Start below
    };
}
```

## Quick Start

### 1. Create a flake.nix

nix-tasks configuration is a [Nix module](https://nixos.org/manual/nixpkgs/stable/#module-system-introduction):
an attribute set (or a function returning one) that sets the `packages`,
`tasks` and `devShells` options. `mkFlakeOutputs` evaluates your modules for
every system and returns the flake outputs the CLI reads.

```nix
{
  description = "My project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ pkgs, ... }: {
          # Packages tasks and shells refer to by name
          packages = {
            go = pkgs.go;
            nodejs = pkgs.nodejs;
            # Optional: make the CLI available in dev shells
            nix-tasks = nix-tasks.packages.${pkgs.stdenv.hostPlatform.system}.default;
          };

          tasks = {
            build = {
              description = "Build the application";
              deps = [ "go" ];
              commands = [ "go build -o bin/app ./cmd/app" ];
            };

            test = {
              description = "Run tests";
              deps = [ "go" ];
              commands = [ "go test ./..." ];
            };

            clean = {
              description = "Clean build artifacts";
              commands = [ "rm -rf bin/" ];
            };
          };

          devShells.default = {
            packages = [ "go" "nodejs" "nix-tasks" ];
            shellHook = ''
              echo "Ready to develop!"
            '';
          };
        })
      ];
    } // {
      # Optional: `nix run . -- <command>`
      inherit (nix-tasks) packages apps;
    };
}
```

`mkFlakeOutputs` takes:

| Argument | Description |
|----------|-------------|
| `nixpkgs` | The nixpkgs flake used for `pkgs`, task shells and builders (required) |
| `modules` | List of nix-tasks modules |
| `systems` | Systems to generate outputs for (default: x86_64/aarch64 Linux and Darwin) |
| `specialArgs` | Extra arguments passed to every module, e.g. `{ inherit inputs; }` |

Modules receive `pkgs`, `lib` (nixpkgs' lib) and `tasksLib` (the nix-tasks
builders, built with your `pkgs`). The flake outputs are `nixTasksConfig`,
`nixTasksShells`, `nixTasksShellDrvs`, `nixTasksBuilds` (read by the CLI) and
`devShells` (for `nix develop`), each per system.

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
# Run via the app re-exported from nix-tasks
$ nix run . -- list

Tasks:
  build                Build the application
  clean                Clean build artifacts
  test                 Run tests
```

#### Option C: In a development shell

```bash
# Enter the dev shell (which includes the nix-tasks package)
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

### 5. Show execution graph

```bash
$ nix run github:redbackthomson/nix-tasks -- graph deploy

deploy
  build
    compile
    lint
  test
    build *
```

The tree shows every task that would execute when running the target. Tasks that appear more than once are marked with `*` and not expanded again.

### 6. Validate configuration

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
  graph <task>     Show execution graph for a task
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

# Run at most 8 tasks at once (the default is the number of CPUs available)
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

Tasks run in the flake's root directory, so `nix-tasks run` behaves the same
from any subdirectory. The root is the directory `nix` itself evaluates: the
nearest directory containing `flake.nix`, searching up from the current
directory (or `-f`) but not past a git repository root (a directory containing
`.git`) or onto another filesystem. A task's `workingDir` and the `.nix-tasks/` output links
are relative to that root. `-f` accepts a relative path such as
`-f examples/demo`; non-path references like `github:owner/repo` are passed to
`nix` unchanged.

Each task starts as soon as all of its dependencies have succeeded, up to `-j`
tasks at a time. If a dependency fails, its dependents are skipped. By default
the first failure stops the run: tasks still running are interrupted and the
rest are skipped. With `--continue-on-error`, independent tasks keep running,
but dependents of a failed task are still skipped. A task with
`continueOnError = true` is different: its failure is allowed, its dependents
run, and it does not fail the run.

Every task in the run ends in one of these states:

| Icon | State | Meaning |
|------|-------|---------|
| `✓` | passed | Succeeded (or was answered from the cache) |
| `✗` | failed | Exited non-zero; fails the run |
| `!` | failed (allowed) | Failed, but has `continueOnError = true`; its output is shown |
| `-` | skipped | Never started: a dependency did not succeed, or the run stopped |
| `⏹` | interrupted | Killed because the run was cancelled |

If exactly one task fails because its command exited non-zero, `nix-tasks run`
exits with that command's exit code (also with `--raw`); any other failure
exits 1.

Ctrl-C (or SIGTERM) stops the run: each running task's whole process tree is
sent SIGTERM, anything still alive 5 seconds later is killed, and `nix-tasks
run` exits 130. Press Ctrl-C again to quit immediately without waiting.

### Development Shells

```bash
# Enter the default shell
$ nix-tasks shell

# Enter a specific shell
$ nix-tasks shell ci

# Run a command in a shell without entering it
$ nix-tasks shell ci --command "go version"
```

`nix-tasks shell` replaces itself with `nix develop`, so the shell's (or the
command's) exit code, signals and job control behave exactly as with
`nix develop`.

### Caching

nix-tasks caches shell task results that declare `inputs`, based on:
- Task commands and configuration
- The task shell's derivation (changes whenever any of its packages does)
- Input file contents

Only successful runs are cached, so a failed task always runs again. Build
tasks are never cached by nix-tasks; Nix's own store makes an unchanged
derivation build instantly.

```bash
# First run executes the task
$ nix-tasks run lint
✓ lint (3.2s)

# Second run uses cache
$ nix-tasks run lint
✓ lint (cached)

# Force re-run
$ nix-tasks run lint --force
✓ lint (3.1s)

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

Task and dev shell names can contain any character except `"`, which `nix`
cannot select in a flake attribute path; `nix-tasks` reports such names as
configuration errors.

Each entry in `tasks` is a typed submodule. Unknown fields and wrongly typed
values (for example `env.PORT = 8080` instead of `"8080"`) are evaluation
errors.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `description` | string | `""` | Shown by `list` and `describe` |
| `type` | `"shell"` or `"build"` | `"shell"` | Shell tasks run commands; build tasks build `derivation` |
| `deps` | list of package names | `[]` | Packages available to the task |
| `depends` | list of `task:<name>` | `[]` | Tasks that must finish first |
| `after` | list of `task:<name>` | `[]` | Run automatically after these tasks |
| `commands` | list of strings | `[]` | Shell commands, run in order |
| `script` | string or null | `null` | A bash script, instead of `commands` |
| `env` | attrs of strings | `{}` | Environment variables |
| `workingDir` | string or null | `null` | Directory, relative to the flake root |
| `continueOnError` | bool | `false` | Don't fail the run when this task fails |
| `inputs` | list of globs | `[]` | Files hashed into the cache fingerprint |
| `noCache` | bool | `false` | Never cache this task |
| `derivation` | package or null | `null` | What a build task builds |

Evaluation fails with a message listing every problem when a task refers to an
unknown task or package, depends on itself, sets both `commands` and
`script`, or is a build task with `commands`, `script`, `inputs` or
`workingDir` (none of which affect `nix build`).

#### Basic task

```nix
tasks.build = {
  description = "Build the app";
  deps = [ "go" ];           # Package dependencies
  commands = [
    "go build -o bin/app ./..."
  ];
};
```

A task's `commands` (joined by newlines) or `script` run with
`bash -e -o pipefail`: the task fails as soon as any command fails, including
any command in a pipeline, so `go test ./... | tee test.log` fails when the
tests do. To tolerate a failing command, append `|| true` to it, or put
`set +o pipefail` at the top of the script to turn pipeline checking off.

#### Task with environment variables

```nix
tasks.deploy = {
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
tasks.deploy = {
  description = "Deploy (after build and test)";
  depends = [ "task:build" "task:test" ];
  commands = [ "kubectl apply -f deploy/" ];
};
```

#### Post-task hooks (after)

Use `after` to run a task automatically whenever another task runs. This is the inverse of `depends` — instead of "I need X before me", it means "whenever X runs, also run me after it."

This is especially useful when extending tasks from a shared module, since you can hook onto an existing task without modifying its definition:

```nix
tasks.post-build-hook = {
  description = "Generate manifests after build";
  after = [ "task:build" ];  # Runs automatically whenever "build" runs
  commands = [ "generate-manifests" ];
};
```

When you run `nix-tasks run build`, both `build` and `post-build-hook` will execute, in that order; if `build` fails, the hook is skipped. After-hooks chain transitively — if another task hooks onto `post-build-hook`, it will also be included.

#### Task with inputs (for caching)

```nix
tasks.build = {
  description = "Build the app";
  deps = [ "go" ];
  commands = [ "go build -o bin/app ./..." ];
  inputs = [ "**/*.go" "go.mod" "go.sum" ];
};
```

### Task Builders

Modules receive `tasksLib`, a set of builders for common task types. A builder
returns only the fields you passed plus its own, so a builder used in one
module combines with definitions of the same task in another. Values a
builder makes up (such as a default description) are `lib.mkDefault`, so any
plain definition replaces them.

`tasksLib` is built from the `pkgs` of the module evaluation. Outside a module,
use `nix-tasks.lib.mkLib pkgs` (or `nix-tasks.lib.<system>`, which uses
nix-tasks' own nixpkgs).

#### mkTask

`mkTask` returns its argument unchanged; a plain attribute set works the same.

#### mkGoTask

```nix
tasks.build = tasksLib.mkGoTask {
  description = "Build Go application";
  pname = "myapp";
  src = ./.;
  vendorHash = null;
};
```

`mkGoTask` creates a build task backed by `buildGoModule`. `pname` and `src`
are required; `version` defaults to `"0.1.0"`, `ldflags` and `tags` to `[]`.
`env` is merged over the default `CGO_ENABLED = "0"` and set on the
derivation. Task fields (`description`, `deps`, `depends`, `after`,
`continueOnError`, `noCache`) are applied to the task; every other argument is
passed to `buildGoModule`. Build tasks are exposed as the flake output
`nixTasksBuilds.<system>.<task>`; `nix-tasks run` builds that attribute and
links each derivation output into `.nix-tasks/<task>/<output>` (e.g.
`.nix-tasks/build/out/bin/myapp`) in the flake's root directory.

#### mkDockerTask

```nix
tasks.docker = tasksLib.mkDockerTask {
  description = "Build Docker image";
  image = "myapp";
  tag = "latest";
  # "docker" is always added to deps, alongside any deps you list
};
```

The generated `docker build` command is a default: setting `commands` here or
in another module replaces it.

#### mkCompoundTask

```nix
tasks.ci = tasksLib.mkCompoundTask {
  description = "Run full CI pipeline";
  tasks = [ "lint" "test" "build" ];
  # Depends on all listed tasks, plus anything in `depends`
};
```

#### Make compatibility

```nix
tasks = {
  legacy-build = tasksLib.compat.make.mkMakeTask { target = "build"; };
} // tasksLib.compat.make.importMakeTargets {
  targets = [ "clean" "dist" ];
  prefix = "make";  # make-clean, make-dist
};
```

### Composing Configuration

Several modules can define the same task or shell; the module system merges
them. This is how a repository customises shared standards: import the
standards module and add a module of your own.

```nix
modules = [
  company-standards.nixTasksModules.default
  ({ lib, ... }: {
    tasks.lint.commands = lib.mkForce [ "golangci-lint run --config .golangci.yml ./..." ];
    tasks.test.commands = lib.mkBefore [ "echo 'setting up'" ];
    tasks.test.env.CGO_ENABLED = "1";
    tasks.publish.depends = lib.mkBefore [ "task:generate-bindings" ];
    devShells.default.packages = [ "nodejs" ];
  })
];
```

| To | Write |
|----|-------|
| Replace a value | `lib.mkForce value` |
| Add list items or script lines first | `lib.mkBefore [ ... ]` |
| Add list items or script lines last | `lib.mkAfter [ ... ]` |
| Append to `deps`, `depends`, `after`, `inputs`, shell `packages` | a plain list |
| Add or change `env` variables | a plain attribute set |
| Clear a value | `lib.mkForce null` (or `[]`) |
| Let other modules override a value without `mkForce` | `lib.mkDefault value` |

`commands` and `script` are deliberately stricter than other lists: two
modules defining them without `mkForce`, `mkBefore` or `mkAfter` is an error
that explains the fix, because silently running both the standard's and the
repository's commands is almost never what was meant. Package lists such as
`deps` concatenate because adding tools is the common intent; duplicates are
removed. Scalar fields such as `description` follow the module system's usual
rule: two different plain values are an error unless one uses `mkDefault` or
`mkForce`.

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

A shell gets the packages of every shell it extends, their `env` (closer
shells win) and their `shellHook`s in order, root first. `env` is passed to
`mkShell` as environment variables only, so it cannot replace `shellHook` or
other `mkShell` arguments. Extending an unknown shell, or a chain of shells
that extends itself, is an evaluation error.

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

`mkFlakeOutputs` generates outputs for x86_64/aarch64 Linux and Darwin. Pass
`systems` to change that:

```nix
nix-tasks.lib.mkFlakeOutputs {
  inherit nixpkgs;
  systems = [ "x86_64-linux" ];
  modules = [ ./nix-tasks.nix ];
}
```

To evaluate a single system yourself, `nix-tasks.lib.evalConfig { pkgs; modules; }`
returns that system's `nixTasksConfig`, `nixTasksShells`, `nixTasksShellDrvs`,
`nixTasksBuilds` and `devShells`, plus the evaluated `config` and `options`.

Upgrading from the `evalConfig`/`extend`/modifier API? See
[docs/migration.md](./docs/migration.md).

## Examples

See the [examples](./examples) directory:

- **[simple](./examples/simple)** - Minimal example with build, test, clean tasks
- **[demo](./examples/demo)** - Full example with 16 tasks, dependencies, and multiple dev shells
- **[dynamic-tasks](./examples/dynamic-tasks)** - Tasks generated from a list of services
- **[company-standards](./examples/company-standards)** - Company-wide standards flake template
- **[app-using-standards](./examples/app-using-standards)** - Repository extending company standards

## Development

```bash
# Enter dev shell (Go and golangci-lint; run the CLI from source)
nix develop

# Run the CLI
go run ./cmd/nix-tasks list

# Test
go test ./...

# Nix library tests
nix flake check

# Lint
golangci-lint run ./...
```

## License

MIT
