# GitHub Actions Output

nix-tasks runs tasks in parallel and people watch CI logs live to see failures
as they happen, so output must keep streaming. GitHub's `::group::` folding can't
contain interleaved output from several tasks, so this design streams live with
readable prefixes and puts the folded, per-task view at the end of the log and
on the job summary page.

## Activation

| Condition | Output |
|-----------|--------|
| `GITHUB_ACTIONS=true` and no `--output` flag | `github` |
| `--output=github` | `github`, anywhere (useful for testing locally) |
| `--output=stream` / `--output=plain` / `--output=tty` / `-v` | that mode, even under GitHub Actions |

In `github` mode colour is on even though stdout is not a TTY, because the
GitHub log viewer renders ANSI colour. `NO_COLOR` still turns it off.

The mode is a reporter (`ui.GitHubReporter`) alongside progress, stream and
plain.

## 1. Colour-coded task prefixes

Every streamed line except workflow commands (see below) is prefixed with
the task's display label, padded to the longest label in the run, and a
separator. Runs with a single task print no prefix.

```
generate │ controller-gen object:headerFile=hack/boilerplate.go.txt paths=./apis/...
lint     │ level=info msg="[runner] Issues before processing: 12, after: 0"
test     │ === RUN   TestReconcile/creates_deployment
generate │ controller-gen crd paths=./apis/... output:crd:dir=package/crds
test     │ --- PASS: TestReconcile/creates_deployment (0.21s)
```

- Each task's prefix and `│` are drawn in one colour; the line content keeps
  whatever colours the tool emitted.
- Colours come from a palette that leaves out the status colours (red, green
  and yellow): cyan, magenta, blue, bright cyan, bright magenta and bright
  blue. They are assigned in sorted task-name order across the tasks in the
  run, so the same pipeline gets the same colours every time and tasks only
  share a colour when a run has more than six.
- Workflow commands a tool prints (any line GitHub reads as
  `::name params::value`, leading whitespace included) are printed without
  the prefix, so tools behave exactly as they would outside nix-tasks:
  linters' `::error` annotations mark the reported lines, `::add-mask::`
  hides a secret in later lines, a tool's `::group::` folds its output.
  nix-tasks keeps its own layout intact by tracking, per task, what a tool
  opened:
  - a `::stop-commands::<token>` the task never resumed is resumed with
    `::<token>::` right after the task's last line;
  - groups the task left open are closed with `::endgroup::` when it
    finishes;
  - an `::endgroup::` from a task with no open group of its own is printed
    with the prefix, as text, so it can't close a group nix-tasks opened.

  The Results replay and the job summary stay inside nix-tasks' own
  `stop-commands` block (section 6), so replayed commands never run twice,
  and values a tool masked are shown there as `***`.

### Display labels for long task names

Task names can be 60+ characters and often differ only in the middle
(`generate-crossplane-provider-aws-ec2-managed-resources-crds` vs
`...-rds-managed-resources-crds`), so truncating them, at either end or in the
middle, makes tasks indistinguishable. Silently rewriting names into something
that looks like a real task name is worse: people see a name they don't
recognise. The label is chosen so that it is either the user's own choice or
visibly an abbreviation:

1. **`label`, if the task sets one.** A new optional task option, for example
   `label = "ec2-crds";`. Standards modules can set labels for shared tasks.
   Duplicate labels within a config are a validation error, and so is a label
   that is another task's name, since both would make two tasks look alike.
2. **The full name, if it is at most 24 characters.**
3. **Otherwise, the name with leading words abbreviated to initials.** Words are
   split on `-`, `_`, `.`, `:` and `/`, separators are kept, and the last two
   words stay whole, in the style of logback's `%logger{n}`. If two tasks in the
   config still produce the same abbreviation, both keep one more word whole
   until they differ. The result depends only on the config, never on which
   tasks are in a particular run, so a task always gets the same label.

```
generate-crossplane-provider-aws-ec2-managed-resources-crds   → g-c-p-a-e-m-resources-crds
generate-crossplane-provider-aws-ec2-managed-resources-mocks  → g-c-p-a-e-m-resources-mocks
generate-crossplane-provider-aws-rds-managed-resources-crds   → g-c-p-a-r-m-resources-crds
test-unit-crossplane-provider-aws-ec2-managed-resources       → t-u-c-p-a-e-managed-resources
lint                                                          → lint
```

Abbreviations may exceed 24 characters; the cap decides only whether to
abbreviate, so nothing is ever cut off.

Making labels discoverable:

- The first time a task appears in a run, its start banner gives the mapping
  when the label differs from the name:
  `▶ g-c-p-a-e-m-resources-crds = generate-crossplane-provider-aws-ec2-managed-resources-crds`.
- Banners, the Results section, the job summary and annotations always use the
  full name; only the streamed-line prefix uses the label.
- `nix-tasks validate` warns about every task whose name is over 24 characters
  and has no `label`, so teams with long names learn about labels before they
  see abbreviations in CI.
- The same labels are used by the local progress display and `--stream` output.

## 2. Inline status banners

When a task starts or finishes, a banner line is printed in the stream with no
prefix, so it stands out while you watch.

```
▶ generate
▶ lint
lint     │ apis/v1/types.go:42:2: ineffectual assignment to err (ineffassign)
━━ ✗ lint failed · exit 1 · 3.1s ━━
generate │ wrote package/crds/example.io_widgets.yaml
━━ ✓ generate · 12.4s ━━
▶ build
━━ ● vet cached · saved 18.2s ━━
━━ ! fmt-check failed, allowed · exit 1 · 0.4s ━━
━━ - deploy skipped · lint failed ━━
```

| State | Marker | Colour |
|-------|--------|--------|
| started | `▶ name` | dim, in the task's prefix colour |
| Succeeded | `✓` | bold green |
| Succeeded (cache hit) | `●` with time saved | bold blue |
| Failed | `✗` with exit code | bold red |
| FailedAllowed | `!` with exit code | bold yellow |
| Skipped | `-` with reason | grey |
| Interrupted | `⏹` | grey |

## 5. Grouped setup

Evaluating the flake and preparing task environments happens before any task
starts, so it can be folded into one group without conflicting with task output.

```
nix-tasks: deploy · 6 tasks · -j 4
::group::Preparing 3 environments (4.8s)
evaluated nixTasksConfig (1.9s)
environment for build, generate: cached environment (0.1s)
environment for lint: built with nix print-dev-env (2.6s)
environment for test: cached environment (0.1s)
::endgroup::
```

Collapsed, this shows as one line: `▸ Preparing 3 environments (4.8s)`.

Each task gets its own shell from its `deps`, so environments are named by
the tasks that share them. A failed environment's error follows the list,
wrapped in `stop-commands`, and only fails the tasks that use it. With no
environments to prepare the group is `Loading configuration`. Only the
github reporter prepares environments up front (it implements
`runner.SetupReporter`); the other modes still prepare each one when its
first task starts, so the live display is not held up.

## 6. Results section with groups

After the run, a results section prints one entry per task, in the order the
graph defines (dependencies first, then name). Each entry is printed whole, one
after another, so groups never interleave.

```
━━━━━━━━━━━━━━━━━━━━━━━━━━ Results ━━━━━━━━━━━━━━━━━━━━━━━━━━
✓ generate · 12.4s
::group::✗ lint · exit 1 · 3.1s
::stop-commands::a1b2c3d4
…all of lint's output…
::a1b2c3d4::
::endgroup::
● vet · cached · saved 18.2s
✓ build · 41.0s
::group::! fmt-check · failed, allowed · exit 1 · 0.4s
…all output…
::endgroup::
- deploy · skipped (lint failed)

✗ 1 failed · 1 allowed failure · 2 passed · 1 cached · 1 skipped · 56.2s
```

Collapsed, the section reads as a status list; expanding a row shows that
task's output with no other task mixed in.

- Failed and allowed-failure tasks get a group with their full output. Passed,
  cached and skipped tasks get a plain status line: their output is already in
  the live stream above, and repeating it would roughly double the log.
- Entries are in dependency order (dependencies first, then by name), so the
  same pipeline always lists tasks in the same place.
- Replayed output has no prefix, so a line starting with `::` could be read as
  a workflow command (for example a stray `::endgroup::` breaking the folding).
  Each replayed body is wrapped in `::stop-commands::<random token>` …
  `::<token>::`, which tells GitHub to treat everything between as plain text.
- Full output is kept per task in a temp file during the run, not in memory,
  because which tasks fail isn't known until they finish.

## 7. Job summary

At the end of the run, markdown is appended to `$GITHUB_STEP_SUMMARY` and shown
on the workflow run's summary page.

```markdown
### nix-tasks: deploy ✗

| Task | Result | Duration | Notes |
|------|--------|---------:|-------|
| generate | ✓ passed | 12.4s | |
| lint | ✗ failed | 3.1s | exit 1 |
| vet | ● cached | — | saved 18.2s |
| build | ✓ passed | 41.0s | |
| fmt-check | ! failed (allowed) | 0.4s | exit 1 |
| deploy | - skipped | — | lint failed |

**1 failed** · 1 allowed failure · 2 passed · 1 cached · 1 skipped · 56.2s total

<details><summary>✗ lint, last 50 lines</summary>

```text
apis/v1/types.go:42:2: ineffectual assignment to err (ineffassign)
1 issues:
* ineffassign: 1
```

</details>
```

One `<details>` block per failed or allowed-failure task. GitHub limits the
summary to 1 MiB per step, so each block is capped at 50 lines and the whole
summary is truncated with a note if it would exceed the limit.

## 9. Error annotations

Each failed task raises one annotation, shown on the run summary and in the PR's
checks:

```
::error title=nix-tasks: lint failed::lint exited with code 1 after 3.1s. Expand "✗ lint" in the Results section of the log.
::warning title=nix-tasks: fmt-check failed (allowed)::fmt-check exited with code 1; continueOnError is set, so the run continued.
```

GitHub shows at most 10 error and 10 warning annotations per step. When a run
has more failures than that, the first 9 get their own annotation and a tenth
says `…and 4 more failed tasks; see the job summary`.

Annotations printed by the tools themselves (see section 1) are not counted
in nix-tasks' budget, but GitHub's limit covers every annotation in the step. A
step whose tools already emitted 10 errors shows none of nix-tasks' own; the
failures are still in the Results section and the job summary.

## Implementation notes

Where the implementation differs from the sketches above:

- Cache hits show `● vet cached` with no time saved: cache entries do not
  record how long the task took.
- Interrupted tasks (the first failure stops the run without
  `--continue-on-error`) get `⏹ name interrupted · 2.0s` banners and Results
  lines, and an `interrupted` count in the totals. They raise no annotation.
- Workflow command properties are escaped as GitHub requires, so the titles
  above are written `title=nix-tasks%3A lint failed`.
- A failure that is not a non-zero exit (for example a broken environment)
  shows its error at the end of its Results group and in its annotation and
  summary row.
- A job summary `<details>` block is titled `✗ lint output` when the whole
  output fits in 50 lines and `✗ lint, last 50 lines` when it was cut.
- A tool's workflow commands are found the way the GitHub runner finds them:
  after leading whitespace, `::`, a command name with optional parameters,
  and the next `::`. While a tool's `stop-commands` block is open GitHub
  ignores every command in the step, other tasks' included, so nix-tasks
  does not count groups opened then; a group another task leaves open in
  that time is closed once the block is resumed.
- Passing commands through means a tool's group also folds whatever other
  tasks print while it is open, and a tool's `stop-commands` silences other
  tasks' annotations until it resumes; that is how the same commands behave
  in any step running tools in parallel.
- Live output keeps the tool's own colour codes even when nix-tasks' colour
  is off, like `--output=stream`; the Results replay strips them in that case.
- With several targets (`nix-tasks run lint build`) the header and the job
  summary title name all of them; prefixes, colours and the Results order
  cover the whole execution set as for one target.
- The Results order and the colour assignment come from the run plan the
  runner reports before any task starts (`runner.PlanReporter`), which also
  carries the labels.

## Decisions

1. Passed tasks get a plain status line in the Results section, with no group.
2. The Results section is in dependency order, not completion order.
3. Allowed failures raise `::warning` annotations; failures raise `::error`.
