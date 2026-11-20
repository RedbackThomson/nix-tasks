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
builders, built with your `pkgs`). The flake outputs, each per system, are
`nixTasksConfig`, `nixTasksShellDrvs` and `nixTasksBuilds` (read by the CLI)
and `devShells` (for `nix develop`).

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

Loading the config already rejects unknown packages, tasks and shells.
`validate` also checks the task graph for dependency cycles and that every
build task resolves to a derivation (without building it), and lists every
problem it finds before exiting non-zero.

## Usage

### Commands

```
nix-tasks <command> [flags]

Commands:
  run <task>...    Run tasks (with dependencies)
  list             List available tasks and shells
  describe <task>  Show task details
  graph <task>     Show execution graph for a task
  shell [name]     Enter a development shell
  cache clean      Remove this repository's cache entries (--task NAME: one task's)
  cache stats      Show cache entries per task
  validate         Validate configuration
  tui              Launch interactive TUI (default; prints the list when piped)
  completions      Generate shell completions

Global Flags:
  --output         How run shows tasks: auto (default), tty, stream, plain, github or json
  -v, --verbose    Same as --output=stream (alias: --stream)
  --color          Colour output: auto (default), always or never
  --debug          Show debug information
  -f, --flake      Path to flake (default: ".")
```

### Interactive TUI

When you run `nix-tasks` without arguments in a terminal, it launches an interactive terminal UI. When stdout is not a terminal (a pipe or a script), it prints the same output as `nix-tasks list` instead:

```bash
$ nix-tasks

nix-tasks  Tasks (3)  Dev Shells (2)
> build  Build the application            │ build
  deploy  Deploy to staging               │ Build the application
  test  Run tests                         │
                                          │ Type         shell
                                          │ Packages     go
                                          │
                                          │ Commands
                                          │   go build -o bin/app ./cmd/app
✓ build passed · 2.3s
enter run · space mark · / filter · g graph · tab shells · ? help · q quit
```

The UI appears straight away and evaluates the flake in the background; if
evaluation fails, the error is shown in the UI. Move with `j/k` or the arrow
keys (the list scrolls to fit the terminal), switch between tasks and dev
shells with `tab`, and press `?` to see every key.

The detail pane, beside the list (or below it in narrow terminals), shows the
selected task's description, type, packages, dependencies, after-hooks, env,
working directory, `continueOnError` and its commands or script; for a dev
shell it shows its packages, the packages it inherits, its full `extends`
chain and its env. `/` filters the list by name or description as you type
(`enter` keeps the filter, `esc` clears it). `g` shows the selected task's
dependency graph, the same tree as `nix-tasks graph`; `esc` returns.

`space` marks tasks (shown with `●`); while any are marked, `enter` runs them
all in one `nix-tasks run a b c`. Marks stay after the run so you can repeat
it, and `esc` clears them (after clearing the filter, if one is set).

`enter` on a task hands the terminal to `nix-tasks run <task>`, so the run
shows its usual progress output; when it finishes, press any key to return.
The status bar then shows how it ended (`✓ build passed · 2.3s`,
`✗ build failed · exit 1 · 3.1s` or `⏹ build interrupted`). `enter` on a dev
shell starts `nix-tasks shell <name>` and returns to the TUI when you exit the
shell. Both run the same nix-tasks binary as the TUI, with its `-f`, `-v`,
`--debug` and `--color` flags. With `--debug`, log lines from loading the
configuration are printed after the TUI exits.

### Running Tasks

```bash
# Run a single task
$ nix-tasks run build

# Run several tasks as one run: shared dependencies run once, one summary
$ nix-tasks run build test lint

# Run with dependencies (automatically runs task:build first)
$ nix-tasks run deploy
✓ build (2.1s)
✓ deploy (0.8s)

# Run at most 8 tasks at once
$ nix-tasks run ci -j 8

# Continue running independent tasks even if one fails
$ nix-tasks run ci --continue-on-error

# Run cached tasks even when nothing changed (refreshes their entries)
$ nix-tasks run generate --force

# Disable the task cache entirely
$ nix-tasks run generate --no-cache

# Stream output in real-time (default in CI; same as -v and --output=stream)
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

A shell task runs its script with the `bash` from its shell, in the
environment `nix develop` would give it (the shell's packages first on
`PATH`), with the task's `env` applied on top. A task's `env.PATH` is put in
front of the shell's `PATH` rather than replacing it, so the shell's tools
stay available; every other variable in `env` replaces the shell's value.
nix-tasks evaluates the shells
of all the tasks in a run at once and realises each distinct shell once, with
`nix print-dev-env`. The resulting environment is cached in
the `devenv/` directory of the nix-tasks cache (`~/.cache/nix-tasks`,
`$XDG_CACHE_HOME/nix-tasks` or `$NIX_TASKS_CACHE_DIR`), keyed
by the shell's derivation, so later runs with the same packages start tasks
without calling Nix for them; a Nix profile link next to each entry stops
`nix store gc` from collecting the shell's packages while the entry exists.
Deleting the directory is always safe. A task with neither `commands` nor
`script`, such as one made with `mkCompoundTask`, needs no shell and finishes
as soon as its dependencies do.

Each task starts as soon as all of its dependencies have succeeded, up to `-j`
tasks at a time. Without `-j`, the limit comes from the `NIX_TASKS_JOBS`
environment variable, and without that it is the number of CPUs available to
nix-tasks. If a dependency fails, its dependents are skipped. By default
the first failure stops the run: tasks still running are interrupted and the
rest are skipped. With `--continue-on-error`, independent tasks keep running,
but dependents of a failed task are still skipped. A task with
`continueOnError = true` is different: its failure is allowed, its dependents
run, and it does not fail the run.

Every task in the run ends in one of these states:

| Icon | State | Meaning |
|------|-------|---------|
| `✓` | passed | Succeeded; `(cached, 41s saved)` when answered from the cache, which the summary counts as `cached` |
| `✗` | failed | Exited non-zero; fails the run |
| `!` | failed (allowed) | Failed, but has `continueOnError = true`; its output is shown |
| `-` | skipped | Never started: a dependency did not succeed, or the run stopped |
| `⏹` | interrupted | Killed because the run was cancelled |

#### Output

`--output` chooses how a run is shown. The default, `auto`, picks a mode from
where the output goes:

- **`tty`, the default on a terminal**: a live region shows each running task
  with the last few lines of its output. Each task's result line is printed
  once, above the region, when it finishes.
- **`github`, the default under GitHub Actions (`GITHUB_ACTIONS=true`)**: see
  [GitHub Actions](#github-actions) below.
- **`stream`, the default in other CI (`CI` is set); also `-v` or `--stream`**: every
  line of task output is printed as it arrives, prefixed with the task name,
  between a start line and a result line. Lines from parallel tasks never
  interleave.
- **`plain`, the default when piped (`nix-tasks run build | tee log`)**: a line
  when each task starts and when it finishes; task output is held back and
  shown only for tasks that fail.
- **`json`, only when asked for**: see [JSON](#json) below.

An explicit `--output` mode wins over `-v` and the environment.

##### JSON

`--output=json` prints nothing while tasks run and then one JSON document on
stdout, for scripts and CI:

```bash
$ nix-tasks run ci --output=json | jq -r '.tasks[] | select(.state == "failed") | .log'
/home/you/.local/state/nix-tasks/runs/20261120T091502.123Z-3fa9c1/test.log
```

```json
{
  "version": 1,
  "targets": ["ci"],
  "flake": "/home/you/repo",
  "system": "x86_64-linux",
  "startedAt": "2026-11-20T09:15:02.123Z",
  "finishedAt": "2026-11-20T09:15:09.480Z",
  "durationMs": 7357,
  "exitCode": 2,
  "error": null,
  "counts": { "passed": 2, "failed": 1, "allowedFailures": 0, "cached": 1, "skipped": 1, "interrupted": 0 },
  "warnings": [],
  "tasks": [
    {
      "name": "test", "label": "test", "state": "failed", "cached": false,
      "afterHook": false, "exitCode": 2, "error": "task 'test' failed: exit status 2",
      "skipReason": null, "startedAt": "2026-11-20T09:15:04.401Z",
      "finishedAt": "2026-11-20T09:15:08.501Z", "durationMs": 4100,
      "log": "/home/you/.local/state/nix-tasks/runs/20261120T091502.123Z-3fa9c1/test.log"
    }
  ]
}
```

- Task output is never in the document. Instead every task that runs writes
  its combined stdout and stderr to its own log, named in `log`:
  `$XDG_STATE_HOME/nix-tasks/runs/<run-id>/<task>.log`, where
  `$XDG_STATE_HOME` defaults to `~/.local/state`. The run ID is the run's
  start time in UTC and a random suffix (e.g. `20261120T091502.123Z-3fa9c1`),
  so run directories sort oldest first, and a `/` in a task name becomes `_`.
  Cached and skipped tasks have no log. Each JSON run removes all but the 20
  most recent run directories. Other output modes write no run logs.
- The document holds the results of the run, not the task definitions.
  `afterHook` marks a task that is in the run because it is an `after` hook
  of another task in it, and was not needed by the targets anyway.
- Tasks are listed in dependency order: each after the tasks it depends on,
  ties broken by name.
- Every field is always present, `null` when it does not apply, so `jq`
  queries need no existence checks.
- The exit code is the same as in every other mode, and is also `exitCode`.
  An error that stops the run before its tasks, such as a task name that
  does not exist, still prints a document, with the message in `error`.
- Warnings, such as a log directory that cannot be created, go to stderr and
  into `warnings`; the tasks still run.
- `--output=json` cannot be combined with `--raw`.

[docs/json-output.md](docs/json-output.md) describes every field and the
versioning rules.

##### GitHub Actions

With `--output=github` (the default when `GITHUB_ACTIONS=true`), output keeps
streaming live so failures show as they happen, and the per-task view comes
at the end:

```
nix-tasks: deploy · 4 tasks · -j 4
▸ Preparing 2 environments (4.8s)
▶ generate
▶ lint
lint     │ apis/v1/types.go:42:2: ineffectual assignment to err (ineffassign)
generate │ wrote package/crds/example.io_widgets.yaml
━━ ✗ lint failed · exit 1 · 3.1s ━━
━━ ✓ generate · 12.4s ━━
━━ ● vet cached ━━
━━ - deploy skipped · lint failed ━━

━━━━━━━━━━━━━━━━━━━━━━━━━━ Results ━━━━━━━━━━━━━━━━━━━━━━━━━━
✓ generate · 12.4s
▸ ✗ lint · exit 1 · 3.1s
● vet · cached
- deploy · skipped (lint failed)

✗ 1 failed · 1 passed · 1 cached · 1 skipped · 16.2s
```

- Each line of task output is prefixed with the task's
  [display label](#display-labels), padded and coloured per task (never red
  or green, which mean failure and success). A run of one task has no prefix.
- Workflow commands in task output (`::error`, `::add-mask::`, `::group::`,
  …) are printed without the prefix, so tools behave as they would without
  nix-tasks. When a task finishes, nix-tasks resumes a `::stop-commands::`
  it left open and closes groups it left open, and an `::endgroup::` with no
  group of the task's own is shown as text, so tool output can't break the
  folding. Values a tool masked are shown as `***` in the Results section
  and the job summary.
- Evaluating the configuration and preparing every task environment happens
  before any task starts, folded into one group.
- The Results section lists every task in dependency order. Failed and
  allowed-failure tasks are folded groups holding their whole output, so it
  can be read without other tasks mixed in.
- Each failed task raises an error annotation and each allowed failure a
  warning. GitHub shows 10 of each per step, counting the tools' own
  annotations too; past that, nix-tasks' last one counts the rest.
- When `GITHUB_STEP_SUMMARY` is set, a table of results and the last 50 lines
  of each failure are appended to the job summary.
- Output is coloured even though stdout is not a terminal, because the
  GitHub log viewer renders colour; `NO_COLOR` or `--color=never` turns it off.

[examples/github-actions](examples/github-actions) has a workflow for checking
the rendering on GitHub, and
[docs/github-actions-output.md](docs/github-actions-output.md) the design.

##### Display labels

Streamed and GitHub output prefix each line with the task's display label,
and the live display names running tasks by it; result lines and the summary always
use the full name. The label is:

1. the task's `label`, if set (for example `label = "ec2-crds";`);
2. the full name, if it is at most 24 characters;
3. otherwise the name with its leading words cut to initials, keeping the
   last two words whole. Words are split on `-`, `_`, `.`, `:` and `/`, and
   the separators are kept. If two tasks still get the same abbreviation,
   both keep one more word whole until they differ.

```
generate-crossplane-provider-aws-ec2-managed-resources-crds  → g-c-p-a-e-m-resources-crds
test-unit-crossplane-provider-aws-ec2-managed-resources      → t-u-c-p-a-e-managed-resources
lint                                                         → lint
```

Labels depend only on the configuration, never on which tasks a run
includes, so a task is always shown the same way. When a task's label
differs from its name, its start line gives both:
`▶ g-c-p-a-e-m-resources-crds = generate-crossplane-provider-aws-ec2-managed-resources-crds`.
Two tasks with the same `label`, or a `label` that is another task's name, is
a validation error, and `nix-tasks validate` warns about every task whose name
is over 24 characters and has no `label`. `nix-tasks describe` shows a task's
label, or its abbreviation when the name is abbreviated.

```
▶ build
▶ test
✓ build (2.3s)
✗ test (4.1s) (exit code 2)
- deploy (skipped)

deploy - (skipped)
├─ build ✓ (2.3s)
│  └─ generate ✓ (cached, 12s saved)
└─ test ✗ (4.1s) (exit code 2)

✗ test output:
    --- FAIL: TestThing
    FAIL

Completed: 4 tasks (1 passed, 1 cached (12s saved), 1 failed, 1 skipped)
```

Every mode ends with the same summary:

1. The dependency tree of each target, each task with its marker, duration
   (or `(cached, 12s saved)` with the duration of the run the cache entry
   came from) and, for a failure, its exit code. A run of several
   targets draws one tree per target, in the order given. A task needed by
   several others, or by several targets, is drawn once; later appearances
   are marked with `*`.
2. The output of each failed or allowed-failed task that was not already
   shown: its last 50 lines. When a real failure's output is longer, the
   full output is saved to a temp file whose path is printed; an allowed
   failure (`continueOnError`) never writes one.
3. The totals.

`--raw` connects the target task directly to stdout and stderr and prints
nothing else, so it takes a single task. If a dependency fails, the summary
is printed to stderr so it is clear why the target did not run.

If exactly one task fails because its command exited non-zero, `nix-tasks run`
exits with that command's exit code (also with `--raw`); any other failure
exits 1.

Output is coloured only when stdout is a terminal (or with `--output=github`)
and `NO_COLOR` is not set; `--color=always` or `--color=never` overrides that
for any command.

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

The task cache is opt-in. A shell task that sets `cache` is skipped when its
declared inputs, its definition and its toolchain are unchanged **and** its
declared outputs are still exactly what the last successful run left. It is
meant for generate-style tasks (controller-gen, mock or client generation,
`go generate`); `go build`, `ko` and `docker` have caches of their own, and
build tasks (`type = "build"`) are never cached by nix-tasks because Nix
already knows whether a derivation needs rebuilding.

```nix
tasks.generate = tasksLib.mkTask {
  deps = [ "go" "controller-gen" ];
  commands = [ "go generate ./..." ];
  cache = {
    inputs  = [ "apis/**/*.go" "go.mod" "go.sum" "hack/boilerplate.go.txt" "!apis/**/testdata/**" ];
    outputs = [ "apis/**/zz_generated.*.go" "package/crds/*.yaml" ];
    env     = [ "GOFLAGS" ];
  };
};
```

| Field | Required | Meaning |
|-------|----------|---------|
| `cache.inputs` | yes, non-empty | Globs of files whose content determines the outputs. `!pattern` excludes. |
| `cache.outputs` | yes (`[]` allowed) | Globs of files the task writes. They are left out of the inputs and checked on every hit. `[]` is for check-only tasks. |
| `cache.env` | no | Environment variables whose values affect the outputs (unset and empty differ). |
| `cache.maxEntries` | no | Entries kept for this task (see below). |

A task is skipped when all of these match a stored entry: `commands`/`script`,
`env`, `workingDir` and the `cache` patterns; the system and the task shell's
derivation (every package version); the values of `cache.env`; the content of
every input file; and the keys of the tasks it depends on. Its outputs in the
working tree must also hash to what that entry recorded, so after a branch
switch, a hand edit or a deleted generated file the task runs again, and
switching back hits the earlier entry.

Patterns:

- are relative to the flake root for every task, whatever its `workingDir`
  and wherever you run `nix-tasks`; absolute paths and `..` are rejected;
- use `**` globs; wildcards skip dot-files and dot-directories (`.git`,
  `.direnv`), but an explicit name such as `.golangci.yml` matches;
- never follow symlinks: a matched symlink is hashed by its target;
- must each match at least one file, otherwise the task fails before running
  (outputs are checked after a successful run). A pattern naming a directory
  matches nothing; write `dir/**`. `.gitignore` is not consulted.

The trust contract: a task still sees the whole working tree and environment,
but only what it declares is hashed. If a file or variable changes your task's
outputs, declare it.

A cached task always runs when a shell task upstream of it (through `depends`
or `after`, transitively) is not cached, since that task could change anything
without the cache seeing it; `nix-tasks validate` and every `run` that
schedules it print a warning. Build tasks upstream are fine: the derivation
path of each one (evaluated, not built) is part of the downstream task's key,
so it hits while the derivation is unchanged and runs again when it changes. It also runs uncached when an upstream
`continueOnError` task failed. Only complete, successful runs are stored:
failures, interrupted runs and runs during which an input file changed are
not. After-hooks of a task still run when it is answered from the cache.
With `--raw`, a target answered from the cache prints `<task>: cached` on
stderr instead of its output.

```bash
$ nix-tasks run generate
✓ generate (41.2s)

$ nix-tasks run generate
✓ generate (cached, 41s saved)

generate ✓ (cached, 41s saved)

Completed: 1 tasks (1 cached (41s saved))

$ nix-tasks cache stats           # entries, size, last hit and last run per task
$ nix-tasks cache clean --task generate
$ nix-tasks cache clean           # every entry for this repository
```

Entries are stored in `v2/` under the nix-tasks cache directory
(`$NIX_TASKS_CACHE_DIR`, `$XDG_CACHE_HOME/nix-tasks` or `~/.cache/nix-tasks`)
and shared by every checkout of a repository (git worktrees, jj workspaces,
colocated or not, and clones of the same `origin`), so a result computed in
one checkout is reused in another whose files match. The repository is found
from git, or by reading the `.jj` directory, so `jj` need not be installed. Each task keeps its 8 most recently used entries;
set `cacheSettings.maxEntriesPerTask` for the whole flake, `cache.maxEntries`
for one task, or `NIX_TASKS_CACHE_MAX_ENTRIES` on a machine (it wins over
both). Entries of tasks that no longer exist are removed after 30 days. The
cache is disabled, with a warning, when `--flake` is not a local path.

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

The scripts only pass the command line to `nix-tasks`, which completes it
from its own CLI definition (with
[kong-completion](https://github.com/jotaen/kong-completion)), so every
command, flag and flag value is covered and the scripts never go stale. Task
and dev shell names are read from `nix-tasks list --names` for the flake given
with `-f` (or the current directory), which evaluates the flake on each TAB;
`-f` itself completes directories. Every argument after `run` completes to a
task name, since `run` takes several. The Nix package installs all three
scripts. In zsh, run `compinit` before adding the script to `~/.zshrc`.

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
| `label` | non-empty string or null | `null` | Short name for streamed output; see [Display labels](#display-labels) |
| `type` | `"shell"` or `"build"` | `"shell"` | Shell tasks run commands; build tasks build `derivation` |
| `deps` | list of package names | `[]` | Packages available to the task |
| `depends` | list of `task:<name>` | `[]` | Tasks that must finish first |
| `after` | list of `task:<name>` | `[]` | Run automatically after these tasks |
| `commands` | list of strings | `[]` | Shell commands, run in order |
| `script` | string or null | `null` | A bash script, instead of `commands` |
| `env` | attrs of strings | `{}` | Environment variables |
| `workingDir` | string or null | `null` | Directory, relative to the flake root |
| `continueOnError` | bool | `false` | Don't fail the run when this task fails |
| `cache` | attrs or null | `null` | Enables the [task cache](#caching) |
| `derivation` | package or null | `null` | What a build task builds |

Evaluation fails with a message listing every problem when a task refers to an
unknown task or package, depends on itself, sets both `commands` and
`script`, or is a build task with `commands`, `script` or `workingDir` (none
of which affect `nix build`). `inputs`, `outputs` and `noCache` were replaced
by `cache`; see [docs/migration.md](docs/migration.md#task-cache).

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

The `task:` prefix in `depends` and `after` is optional: `"build"` and
`"task:build"` name the same task.

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

#### Cached task

```nix
tasks.mocks = {
  description = "Generate mocks";
  deps = [ "go" "mockgen" ];
  commands = [ "go generate ./internal/..." ];
  cache = {
    inputs = [ "internal/**/*.go" ];
    outputs = [ "internal/**/mock_*.go" ];
  };
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
derivation. Task fields (`description`, `label`, `deps`, `depends`, `after`,
`continueOnError`) are applied to the task; every other argument is
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
| Append to `deps`, `depends`, `after`, `cache.inputs`, `cache.outputs`, `cache.env`, shell `packages` | a plain list |
| Disable a standard's cache for a task | `tasks.<t>.cache = lib.mkForce null` |
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
returns that system's `nixTasksConfig`, `nixTasksShellDrvs`, `nixTasksBuilds`
and `devShells`, plus the evaluated `config` and `options`.

Upgrading from the `evalConfig`/`extend`/modifier API? See
[docs/migration.md](./docs/migration.md).

## Examples

See the [examples](./examples) directory:

- **[simple](./examples/simple)** - Minimal example with build, test, clean tasks
- **[demo](./examples/demo)** - Full example with 16 tasks, dependencies, and multiple dev shells
- **[dynamic-tasks](./examples/dynamic-tasks)** - Tasks generated from a list of services
- **[company-standards](./examples/company-standards)** - Company-wide standards flake template
- **[app-using-standards](./examples/app-using-standards)** - Repository extending company standards
- **[github-actions](./examples/github-actions)** - Pipeline and workflow for checking the GitHub Actions output

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
