# Task Cache Redesign

An opt-in cache that lets nix-tasks skip a shell task when its declared inputs,
its definition and its toolchain are unchanged, and its declared outputs are
still in the state the last successful run left them.

Related: [requirements.md](./requirements.md) (section 9, Caching Strategy).

## 1. Goals and Non-Goals

**Goals**

- Skip expensive generate-style tasks (controller-gen, CRD/client generation,
  mock generation, `go generate`) when nothing they depend on has changed.
- Never produce a false hit for a correctly declared task. A miss costs one
  extra run; a false hit ships stale generated code.
- Opt-in per task. No task is cached unless it says so.
- No Nix derivations required from users. Tasks stay plain shell commands.
- Explainable: a user can always find out why a task missed.

**Non-goals**

- Remote or shared caching between machines or CI runners.
- Restoring outputs. v1 never writes to the workspace on a hit; it only skips
  work when the outputs are already correct. See [Future: output restore](#future-output-restore).
- Caching `go build`, `ko`, `docker build` or `docker push`. They have their own
  caches, and pushes are side effects that must happen every time.
- Caching build tasks (`type = "build"`). Nix already decides whether a
  derivation needs rebuilding; a second cache on top only adds ways to be wrong.
- Sandboxing. The task still sees the full workspace and environment; the cache
  trusts the declaration (see [Trust contract](#trust-contract)).

## 2. Configuration

### Nix schema

```nix
generate = lib.mkTask {
  deps = [ "go" "controller-gen" ];
  commands = [ "go generate ./..." ];
  cache = {
    inputs  = [ "apis/**/*.go" "go.mod" "go.sum" "hack/boilerplate.go.txt" "!apis/**/testdata/**" ];
    outputs = [ "apis/**/zz_generated.*.go" "package/crds/*.yaml" ];
    env     = [ "GOFLAGS" ];
  };
};
```

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `cache` | attrset or `null` | no (default `null`) | Presence enables caching for the task. |
| `cache.inputs` | list of glob | yes, non-empty | Files whose content determines the outputs. `!pattern` excludes. |
| `cache.outputs` | list of glob | yes, may be `[]` | Files the task writes. Excluded from inputs; verified on hit. `[]` is for check-only tasks (e.g. a schema linter). |
| `cache.env` | list of names | no (default `[]`) | Environment variables whose values affect the outputs. |

`outputs` must be written out even when empty so that "this task produces
nothing" is a stated decision rather than an omission.

### Go types

```go
type Task struct {
    // ...existing fields...
    Cache        *TaskCache `json:"cache,omitempty"`
    ShellDrvPath string     `json:"shellDrvPath,omitempty"`
}

type TaskCache struct {
    Inputs  []string `json:"inputs"`
    Outputs []string `json:"outputs"`
    Env     []string `json:"env"`
}
```

`Task.Inputs` and `Task.NoCache` are removed. `Task.Outputs` stays as the
build-task output map and is no longer accepted from shell tasks.

### Validation (`config.Validate`, also surfaced by `nix-tasks validate`)

| Rule | Severity |
|------|----------|
| `cache` set on a build task | error |
| `cache` set on a task with no `commands` and no `script` (compound tasks) | error |
| `cache.inputs` empty | error |
| Pattern is absolute, contains `..`, or fails `doublestar.ValidatePattern` | error |
| `cache.env` entry is not a valid variable name | error |
| `cache.env` names a variable also set in `task.env` | warning (the literal is already hashed) |
| Input pattern begins with `**` | warning (walks the whole tree) |
| Pattern ends in `/` | warning (directories contribute nothing; use `dir/**`) |
| `ShellDrvPath` missing on a cached task | error (Nix library too old) |
| A cached task has an uncached task anywhere upstream (`depends` or `after`, transitively) | warning; the task always runs (see [Eligibility](#5-hit-and-store-algorithm)) |

Pattern-matches-nothing is checked at run time, not here, because it depends on
the working tree.

### Composition

The deep merge in `lib/compose.nix` already recurses into attrsets, so the
markers work without changes:

```nix
tasks.generate.cache.inputs = append [ "hack/extra.txt" ];
tasks.generate.cache.env    = override [ ];
tasks.generate.cache        = override null;   # disable caching from a standard
```

`tasks.generate.cache = null` does **not** disable caching: `mergeAttrs` treats a
`null` overlay as "keep the base value". This is documented next to the example.

Modifiers in `lib/modifiers.nix`:

| Modifier | Effect |
|----------|--------|
| `setCache { inputs; outputs; env ? []; }` | Enable or replace the cache block |
| `disableCache` | `cache = null` |
| `appendCacheInputs` / `overrideCacheInputs` | Edit `cache.inputs`; throws if `cache` is `null` |
| `appendCacheOutputs` / `overrideCacheOutputs` | Edit `cache.outputs`; throws if `cache` is `null` |
| `appendCacheEnv` | Edit `cache.env`; throws if `cache` is `null` |

The append modifiers throw on an uncached task rather than create a cache block,
because a half-declared cache (inputs but no thought-through outputs) is exactly
what produces false hits. `appendInputs`, `overrideInputs` and `setNoCache` are
kept as stubs that throw a message naming the replacement (see
[Migration](#8-migration)).

## 3. Cache Key

### Components

The key is built from five component digests. Keeping them separate (rather than
streaming everything into one hash) is what makes `--explain` possible: each
stored entry records its component digests, and a miss is explained by the ones
that differ.

| Component | Contents | Why |
|-----------|----------|-----|
| `definition` | `commands`, `script`, `env` (literal map), `workingDir`, `cache.inputs`, `cache.outputs`, `cache.env` | Anything that changes what the task does or what it claims to read/write. Patterns are included so that narrowing an exclusion invalidates. `description`, `deps` names, `continueOnError`, `after`, `type` are excluded: they don't change the outputs, and `deps` is covered more precisely by `toolchain`. |
| `toolchain` | `system`, `shellDrvPath` | The task runs inside `mkShell`, so the drv covers every package version, stdenv, and setup hooks. Package store paths alone (today's approach) miss stdenv and silently drop unknown names. |
| `env` | For each name in `cache.env`, sorted: name, set/unset flag, value | The flag distinguishes unset from empty. Values are hashed into the key but never written to the store in clear (they can be tokens). |
| `inputs` | Sorted list of (flake-root-relative path, file kind, sha256 of content) for files matched by `inputs`, minus `!` exclusions, minus files matched by `outputs` | Content, not mtime, so `git checkout` back and forth hits. Excluding outputs stops generated files that live beside sources (`zz_generated.*.go` under `apis/**/*.go`) from invalidating their own entry. |
| `upstream` | Sorted list of (task name, upstream digest) for each direct graph edge (`depends` and `after` targets) | Every upstream is itself cached (a task with an uncached upstream is never eligible), so each contributes its full key and a change anywhere up the chain propagates. |

`key = sha256(frame("nix-tasks-cache", formatVersion) ‖ frame("definition", d) ‖ frame("toolchain", t) ‖ frame("env", e) ‖ frame("inputs", i) ‖ frame("upstream", u))`

### Canonical encoding

Every digest is computed over a framed byte stream, never over concatenated
strings or `encoding/json` output (map ordering and escaping are not a contract):

- `frame(tag, value)` = `uvarint(len(tag)) ‖ tag ‖ uvarint(len(value)) ‖ value`.
- A list is `uvarint(count)` followed by framed items; a map is its entries
  sorted by key, each as `frame(key, value)`.
- Optional strings are framed with a leading presence byte (`0` absent, `1`
  present) so that `script = null` and `script = ""` differ.
- File entries are `frame(path) ‖ frame(kind) ‖ frame(sha256(content))` where
  `kind` is `file` or `symlink`. The content hash is fixed-length, so no path and
  content can collide (the `a`+`bc` vs `ab`+`c` problem).

`formatVersion` starts at `2` and lives in `internal/cache/key.go`. Any change to
what goes into a component or how it is encoded bumps it. Old entries become
unreachable and are removed by eviction or `cache clean`.

### Trust contract

The process still receives the full inherited environment, because `go generate`
needs `HOME`, `GOPATH`, `GOCACHE` and friends. Only `cache.env` is hashed. The
same applies to files: the task can read anything, but only `cache.inputs` is
hashed. The documentation states this as a contract: *if a file or variable
changes your task's outputs, declare it.* `--explain` and the run-time checks in
§5 make violations visible rather than prevent them.

## 4. Glob Semantics

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Root | Flake root (the directory containing `flake.nix`), for every task regardless of `workingDir` | One mental model across tasks and across standards imported from another flake. Keys are independent of where the user invoked `nix-tasks`. A `workingDir`-relative root would make the same pattern mean different things in a composed config. |
| Non-local flakes | Caching is disabled with a warning when `--flake` is not a local path | There is no working tree to hash. |
| `..` and absolute paths | Rejected by validation | Keeps inputs inside the tree the key describes. |
| Hidden files | Wildcards do not match dot-files or descend into dot-directories (`doublestar.WithNoHidden`); explicit names like `.golangci.yml` still match | Excludes `.git`, `.jj`, `.direnv` without a special list. |
| `.gitignore` | Not consulted | Ignored files are often genuine inputs (tool binaries in `bin/`, local `.env`), honouring it needs either a full gitignore implementation or a `git` subprocess, and global excludes make keys machine-dependent. Users write narrow patterns and use `!` exclusions. |
| Exclusions | `!pattern` entries in `inputs`; directories that can only match exclusions are pruned during the walk (`fs.SkipDir`) | Cheap way to keep `vendor/` or `node_modules/` out without a second field. |
| Symlinks | Not followed (`WithNoFollow`). A matched symlink is hashed as kind `symlink` with its target string as content | Following links can escape the root or loop. Nix `result` links are content-addressed, so hashing the target is exact. |
| Directories | A pattern matching a directory contributes nothing; only files are hashed | `dir/` would otherwise hash nothing silently. Validation warns on patterns ending in `/`. |
| Zero matches | Any input or output pattern (other than `!` exclusions) that matches no file is a task error before the command runs; output patterns are checked after a successful run | A typo makes the cache blind to a whole class of change. Failing loudly is the only safe response, and the user opted into caching. |
| I/O errors | Unreadable file or directory fails the task (`WithFailOnIOErrors`) | Silently skipping is how the current cache misses changes. |

### Performance

Inputs are hashed twice on a miss (before the run, and again after to detect
changes during the run) and once on a hit; outputs once each way. For the target
workload (a few thousand Go files, tens of MB) sha256 runs well under a second,
small next to `nix develop` start-up. A stat-based fast path keyed on
(size, mtime, inode, ctime) is **not** in v1: it adds a second source of truth
with known races (files modified within the mtime granularity of the previous
hash), and there is no evidence yet that hashing dominates. v1 logs hashing
time per task at debug level and in `--explain`; the entry format reserves a
`stat` field so a fast path can be added without a format bump if the numbers
justify it.

## 5. Hit and Store Algorithm

For each task the scheduler starts, `runTask` does:

1. **Eligibility.** The task is cache-eligible iff it has `cache`, `--no-cache` is
   not set, the flake is local, every task upstream of it (transitively) also
   has `cache`, and every upstream task finished as `Succeeded` (ran or hit).
   An uncached upstream can change anything without nix-tasks seeing it, so a
   task downstream of one always runs. `validate` warns about this, and so does
   every `run` that schedules such a task, before any task starts:
   `warning: generate has cache enabled but always runs: it depends on tools-install, which is not cached`.
   If any upstream is `FailedAllowed`, the task runs uncached: its upstream's
   products may be partial.
2. **Key.** Compute the five components. Upstream keys come from a
   `map[string]string` on the executor, written under a mutex when each task
   finishes, so a task skipped by a hit still publishes its key. Pattern errors
   fail the task here.
3. **Lookup** (skipped with `--force`). Read every entry for this task whose key
   matches. Hit iff one exists **and** the current output digest equals the one
   it recorded. A hit touches the entry's `lastUsed` and returns
   `Succeeded{Cached: true}`.
4. **Run** the task normally.
5. **Store** only if the result is `Succeeded` and the context is not cancelled.
   Re-hash inputs; if they differ from step 2, log a warning naming the changed
   files and do not store (the run may have seen a mix, or the task writes to its
   own inputs, which would otherwise miss forever). Hash outputs, then write the
   entry atomically. A store failure is a warning, never a task failure.

| Situation | Behaviour |
|-----------|-----------|
| Task fails | Not stored. Nothing is ever replayed as "failed (cached)". |
| `continueOnError` task fails | Result is `FailedAllowed`; not stored; dependents run uncached. |
| Ctrl-C / fail-fast cancellation | Result is `Interrupted` or `Skipped`; not stored. |
| Hit | Dependents see `Succeeded`, receive the task's key, and may themselves hit. |
| After-hooks on a hit target | Still run (a hit is a success). A hook that should only fire on real work must not be attached via `after`; noted in docs. |
| `--force` | No lookup; stores on success, refreshing the entry. |
| `--no-cache` | No hashing, lookup or store. Pattern errors are not raised. |
| `--raw` target hits | Prints `<task>: cached` to stderr so the user isn't left with silent output. |
| Two processes, same task | Each may run; both stores are atomic; last rename wins with equivalent content. |

Output verification is what makes this safe without restore: after a branch
switch the committed generated files change, so the recorded output digest no
longer matches and the task reruns; switching back finds the earlier entry
(several entries are kept per task) with matching outputs and hits. A hand edit
or deletion of a generated file is a mismatch, so the task reruns and
regenerates it.

## 6. Store

### Layout

```
$NIX_TASKS_CACHE_DIR | $XDG_CACHE_HOME/nix-tasks | ~/.cache/nix-tasks
└── v2/
    └── <projectKey>/                     # see below
        └── <taskKey>/                    # sha256(task name)[:16]; names with "/" are safe
            └── <cacheKey>.json
```

The cache is shared by every checkout of a repository (git worktrees, jj
workspaces, separate clones), so parallel agents working in their own worktrees
reuse each other's results. That is safe because nothing in the key depends on
where the checkout lives: input paths are flake-root-relative, and a hit still
requires the outputs in *this* checkout to match. `projectKey` only groups
entries for eviction and `cache stats`. It is `sha256` of the first of these that
exists: the normalised `origin` remote URL, the git common directory (shared by
all worktrees of one clone), or the absolute flake path.

The directory is versioned (`v2/`) so the current flat layout can be removed
wholesale and so a future incompatible layout can sit beside this one.

### Entry

```json
{
  "format": 2,
  "task": "generate",
  "key": "…",
  "created": "2026-11-19T10:00:00Z",
  "lastUsed": "2026-11-19T12:00:00Z",
  "runDuration": "41.2s",
  "components": { "definition": "…", "toolchain": "…", "env": "…", "inputs": "…", "upstream": "…" },
  "detail": {
    "shellDrvPath": "/nix/store/…-nix-shell.drv",
    "env": { "GOFLAGS": "sha256:…" },
    "upstream": { "tools": "…" },
    "inputs": [["apis/v1/types.go", "file", "sha256…"]],
    "outputs": [["apis/v1/zz_generated.deepcopy.go", "file", "sha256…"]]
  },
  "outputDigest": "…"
}
```

`detail` holds just enough to diff against the current state; env values are
stored only as hashes.

### Atomicity

Write to `<cacheKey>.json.tmp-<pid>-<rand>` in the same directory, `fsync`,
`rename`. Readers treat unparseable or wrong-format files as absent and delete
them. `lastUsed` updates use the same write path.

### Eviction

On every store, keep the most recently used entries for that task (8 by
default) and delete the rest. Enough for several branches in flight without
unbounded growth. The limit is configurable in two places, the more specific
winning:

| Where | Setting | Use |
|-------|---------|-----|
| Flake config | `cacheSettings.maxEntriesPerTask = 16;` (top-level option, settable from a standards module) | A repo with many long-lived branches. |
| Task | `cache.maxEntries = 32;` | One task that flips between many states. |
| Environment | `NIX_TASKS_CACHE_MAX_ENTRIES=4` | A machine with little disk, or CI. Overrides both. |

There is no user-level config file today, and this doesn't justify adding one.
Concurrent processes can evict an entry another has just written; the only cost
is a later miss. Entries
for tasks that no longer exist are removed by `cache clean` or, opportunistically,
on any run that loads the config (entries whose `task` is not in the config and
whose `lastUsed` is older than 30 days).

### Commands

| Command | Behaviour |
|---------|-----------|
| `cache clean` | Remove this project's `v2/` entries and any legacy flat entries. |
| `cache clean --task NAME` | Remove one task's entries. |
| `cache stats` | Per task: entries, size, last hit, last run duration; totals. |

### Explaining a miss

`nix-tasks run --explain` prints, after the summary, one block per cache-enabled
task in the run:

```
generate: miss
  inputs changed: 2 modified, 1 added
    M apis/v1/types.go
    M go.sum
    A apis/v1/new_types.go
  upstream changed: tools
codegen-mocks: hit (saved 12.4s)
crds: miss — outputs changed since last run
    M package/crds/foo.yaml
```

The comparison is against the most recently used entry for the task. Components
are reported in key order; file lists are truncated at 20 lines. When no entry
exists the reason is `no previous entry`. The same one-line reason is logged at
debug level on every miss, so `--debug` users get it without the flag.

## 7. UX

- Progress display and tree output keep the existing `✓ name (cached)` line and
  add the saved time from `runDuration`: `✓ generate (cached, 41s saved)`.
- The summary gains a count: `Completed: 5 tasks (3 passed, 2 cached)`.
- `describe` gains a `Cache:` section listing inputs, outputs and env, or
  `Cache: disabled`. It does not hash anything; current hit/miss status is
  `run --explain`'s job.
- `graph` labels cached tasks the same way the tree output does.

## 8. Migration

This is a hard break for the fields whose meaning changes, and a warning for the
one that becomes a no-op. `mkShellTask` takes `...`, so a removed argument would
otherwise be swallowed silently and users would believe a task is cached when it
isn't.

| Old | New behaviour |
|-----|---------------|
| `inputs` on `mkTask`/`mkShellTask` | Nix `throw`: "`inputs` moved to `cache.inputs`; `cache.outputs` is now required". |
| `outputs` on shell tasks | Nix `throw` with the same pointer. Fixes the list-vs-map decode failure. |
| `noCache` on any builder | `lib.warn` and ignored; caching is off unless `cache` is set. |
| `appendInputs` / `overrideInputs` / `setNoCache` | Throw, naming the replacement modifier. |
| Raw task attrsets passed to `evalConfig` | `types.validateConfig` applies the same checks, so hand-written tasks can't bypass them. |
| Build-task caching | Removed. Build tasks always run `nix build` (a no-op when the store path exists) and always re-link outputs. |
| Legacy cache files | Ignored; removed by `cache clean`. |

Files to update: `README.md` (inputs/outputs example around line 450, modifier
table), `docs/requirements.md` section 9, `examples/demo/flake.nix` (shell tasks
with `inputs`/list `outputs`), `tests/integration/testdata/simple/flake.nix`
(`hello` uses `inputs`), `lib/{builders,types,modifiers,default}.nix`,
`internal/cache/cache_test.go`, and the cache tests in
`tests/integration/integration_test.go`. The demo's `build-app` and
`build-frontend` lose caching entirely; the demo gains a `generate` task showing
the intended use.

## 9. Prerequisites

| Planned work | What the cache needs | Blocking? |
|--------------|----------------------|-----------|
| (c) Resolve paths against the flake root | `workingDir` and glob roots must be the same directory the command actually runs in. Today `cmd.Dir = task.WorkingDir` resolves against the process cwd, so the files hashed and the files read could differ. | **Yes.** Land first. |
| (d) Export task shell `drvPath` | `nixTasksConfig.tasks.<name>.shellDrvPath = nixTasksShells.<name>.drvPath`, emitted only when `task.cache != null` so config load does not instantiate a shell per task. Whether tasks later run via `nix print-dev-env` is orthogonal; the key uses the same drv either way. | **Yes** for the `toolchain` component. Small; can ship with the cache. |
| (b) Explicit result states | Store only on `Succeeded`; gate eligibility on upstream `Succeeded`; report hits as `Succeeded{Cached}`. Today `continueOnError` failures are turned into `nil` errors inside `runTask` and are indistinguishable from success downstream. | **Yes.** Land before or with the cache. |
| (a) Ready-queue scheduler | A task's key must be published before any dependent starts. Both the level-based and ready-queue schedulers guarantee this; the cache only needs a shared key map. | No. Either order works; the ready queue makes hits pay off sooner (a hit frees its dependents immediately rather than at the end of the level). |

Order: (c) → (b) → the cache (including (d)) → (a) whenever ready.

## 10. Test Plan

### Unit (`internal/cache`)

One table-driven test per key component, each asserting the key changes, plus
negative cases asserting it does not:

| Change | Key changes? |
|--------|--------------|
| commands, script (incl. `null` vs `""`), literal env, workingDir | yes |
| any `cache.inputs` / `outputs` / `env` pattern | yes |
| description, continueOnError, after-hook list order | no |
| shellDrvPath, system | yes |
| `cache.env` value; unset vs empty | yes |
| undeclared env var | no |
| input file content; file added; file removed; file renamed | yes |
| file matching both inputs and outputs is edited | no |
| `!`-excluded file edited | no |
| symlink retargeted | yes |
| upstream cached task key; upstream uncached task's commands | yes |
| path/content boundary (`a`+`bc` vs `ab`+`c`) | yes (distinct) |
| map iteration order, cwd, absolute flake location | no |

Glob tests: root is flake root regardless of cwd and `workingDir`; zero-match
input or output errors; `..` and absolute rejected; hidden dirs skipped;
symlinked dir not traversed; unreadable file errors.

Store tests: interrupted write (temp file left behind) is never read as an
entry; corrupt entry treated as miss and removed; concurrent stores of the same
key from goroutines leave one valid file; eviction keeps the 8 most recently
used; task names with `/` stay inside the task directory.

Explain tests: diff output for each component; file list truncation.

### Unit (`internal/runner`)

With a fake executor: failure not stored; `continueOnError` failure not stored
and dependents ineligible; cancelled context not stored; hit publishes key to
dependents; `--force` stores; `--no-cache` neither reads nor writes nor errors on
bad patterns; inputs modified during the run → not stored.

### Integration (`tests/integration`, new `testdata/cache` flake)

The flake has a `generate` task that concatenates `src/*.txt` into `gen/out.txt`
and appends a line to a counter file, so tests can count real executions.

| Test | Assertion |
|------|-----------|
| Second run hits | counter unchanged, `(cached)` shown |
| Input edit | reruns |
| Branch-switch simulation | A → B → A: third run hits without executing |
| Hand-edited output | reruns, output regenerated |
| Deleted output | reruns |
| Failing task | second run executes again, no "failed (cached)" |
| Ctrl-C (send SIGINT mid-run) | next run executes |
| Upstream cached task's input changes | both upstream and downstream rerun |
| `--explain` after an input edit | names the edited file |
| Old `inputs` field | Nix evaluation error mentions `cache.inputs` |

## Future: output restore

Storing output file contents (content-addressed blobs under the cache dir) would
let a hit write outputs back rather than require them to already match. For
generate tasks whose outputs are committed this rarely helps, because a branch
switch already brings the right generated files. It matters for uncommitted
outputs (e.g. generated mocks in `.gitignore`). Restore also means nix-tasks
writes into the workspace on a hit, which raises questions about partial
restores and concurrent editors. The v1 entry records per-file output hashes, so
blob storage can be added later without changing the key.

## Decisions

1. **Zero-match patterns** fail the task before its command runs. No optional
   patterns in v1.
2. **Uncached upstream tasks** make a cached task always run, with a warning
   from `validate` and at the start of every `run` that schedules it.
3. **One cache per repository**, shared across worktrees, workspaces and clones.
4. **After-hooks** run when their target is a cache hit.
5. **Retention** is 8 entries per task by default, configurable per flake, per
   task and per machine; 30-day orphan cleanup; no global size cap in v1.
6. **Check-only tasks** (`outputs = []`) may be cached.

## Open Questions

1. The composition examples in [Composition](#composition) use the current
   `extend` markers and modifiers. The module-system migration replaces those
   (`lib.mkForce null` disables a standard's cache; list options concatenate);
   this section is rewritten once that lands.
