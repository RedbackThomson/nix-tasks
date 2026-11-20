# JSON output

`nix-tasks run --output=json` prints one JSON document on stdout when the run
finishes, and nothing else on stdout. It is a stable contract for scripts and
CI; the integration tests assert run results through it. It holds the
results of the run only: what can be read from the task definitions, such as
a task's `depends` or `after`, is left to `nix-tasks describe` and the
configuration.

## Rules

- **One document.** Nothing is printed while tasks run. Warnings and errors
  also go to stderr, as `nix-tasks: warning: …` and `nix-tasks: error: …`.
- **No task output.** Every task that runs writes its combined stdout and
  stderr to its own log, named in `log`. Logs live in
  `$XDG_STATE_HOME/nix-tasks/runs/<run-id>/<task>.log` (`$XDG_STATE_HOME`
  defaults to `~/.local/state`); each JSON run removes all but the 20 newest
  run directories. Only `--output=json` writes these logs.
- **Every field is present.** A field that does not apply is `null`, and a
  list with nothing in it is `[]`, never missing.
- **Same exit code.** The process exits with the code any other output mode
  would, and the document repeats it in `exitCode`.
- **Errors before the tasks still produce a document**, with `error` set and
  `tasks` empty: an unknown task name, a configuration that fails to
  evaluate, or a dependency cycle. Errors in parsing the command line (an
  unknown flag, `--output=json` with `--raw`) print no document.
- **Versioning.** `version` is 1. It changes only when a field is removed or
  its meaning changes; new fields can be added without changing it, so
  decoders should ignore fields they do not know.

## Document

| Field | Type | Meaning |
|-------|------|---------|
| `version` | number | Schema version, `1` |
| `targets` | list of strings | The tasks named on the command line, in order |
| `flake` | string | Absolute path of the flake's directory, or the flake reference as given when it is not a local path |
| `system` | string | Nix system, e.g. `x86_64-linux` |
| `startedAt` | timestamp | When nix-tasks started the run |
| `finishedAt` | timestamp | When the document was written |
| `durationMs` | number | `finishedAt` − `startedAt` in milliseconds |
| `exitCode` | number | The process exit code |
| `error` | string or null | Why the run failed outside its tasks; `null` when it did not |
| `counts` | object | Tasks by outcome; see below |
| `warnings` | list of strings | Problems that did not fail the run, e.g. a log directory that could not be created |
| `tasks` | list of task objects | Every task in the run, in dependency order |

Timestamps are RFC 3339 in UTC with exactly three fractional digits
(`2026-11-20T09:15:02.100Z`), so they compare correctly as strings.

`counts` has `passed`, `failed`, `allowedFailures`, `cached`, `skipped` and
`interrupted`. A cache hit counts as `cached`, not `passed`, so the counts
add up to the number of tasks.

## Task

| Field | Type | Meaning |
|-------|------|---------|
| `name` | string | Task name |
| `label` | string | Display label used in streamed output; the name when it has none |
| `state` | string | `succeeded`, `failed`, `failed_allowed` (failed with `continueOnError`), `skipped` or `interrupted` |
| `cached` | bool | The result came from the cache; the task did not run |
| `afterHook` | bool | The task is in the run because it runs `after` another task in the run. `false` for the targets and everything they depend on, including a task that is both a dependency and an after-hook, since it would have run anyway; also `false` for the tasks a hook depends on, which are dependencies |
| `savedMs` | number or null | For a cache hit, how long the run that stored the entry took; otherwise `null` |
| `cache` | object or null | What the [task cache](./cache-redesign.md) did; `null` for a task without a `cache` declaration. See below |
| `exitCode` | number or null | `0` for a task that ran and succeeded; the process exit code of a failure; otherwise `null` (cached, skipped, interrupted, or a failure that was not a process exit, such as a shell that failed to evaluate) |
| `error` | string or null | The error of a failed or interrupted task |
| `skipReason` | string or null | Why a skipped task never started, e.g. `"lint failed"` or `"run stopped"` |
| `startedAt` | timestamp or null | When the task started; `null` when skipped |
| `finishedAt` | timestamp or null | When it finished; `null` when skipped |
| `durationMs` | number or null | How long it took; `null` when skipped |
| `log` | string or null | Path of its log; `null` when it did not run (cached or skipped) or its log could not be created |

`cache` has three fields:

| Field | Type | Meaning |
|-------|------|---------|
| `enabled` | bool | The cache was consulted for this task (looked up, or skipped by `--force`) |
| `hit` | bool | Same as the task's `cached` |
| `reason` | string or null | Why the task ran instead of hitting, e.g. `"no previous entry"`, `"inputs changed: 1 modified"`, `"depends on uncached tools"`, `"--force"` or `"cache disabled"` (`--no-cache`, or a flake that is not a local path); `null` on a hit and for a task that never started |

Tasks are listed with every task after the tasks it depends on (including
the task an `after` hook follows), ties broken by name, so the same pipeline
always lists the same way.

## Examples

```bash
# Names of failed tasks
nix-tasks run ci --output=json | jq -r '.tasks[] | select(.state == "failed") | .name'

# The full output of every failure
nix-tasks run ci --output=json | jq -r '.tasks[] | select(.state == "failed") | .log' | xargs cat

# Slowest tasks
nix-tasks run ci --output=json | jq -r '.tasks[] | select(.durationMs != null) | "\(.durationMs)\t\(.name)"' | sort -rn | head
```
