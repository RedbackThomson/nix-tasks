# GitHub Actions output

A small pipeline for checking how `nix-tasks run` looks in a GitHub Actions
log. It has parallel tasks that print slowly, so their output interleaves; a
long task name with a `label` and one without; a task that prints colour, a
`::warning` annotation (which should appear on the run's summary), a group of
its own, a masked token (which should show as `***`) and a stray
`::endgroup::` (which should show as text); an allowed failure (`fmt-check`); and a `lint` task that
fails when `FAIL_LINT=true`, which makes `deploy` skip. The workflow passes
`--continue-on-error` so the other tasks finish instead of being interrupted.

To see it on GitHub, copy `workflow.yml` into `.github/workflows/` of a fork
and run it from the Actions tab. To preview the log locally:

```bash
FAIL_LINT=true GITHUB_STEP_SUMMARY=/tmp/summary.md nix run ../.. -- run deploy --continue-on-error --output=github
cat /tmp/summary.md
```

See [docs/github-actions-output.md](../../docs/github-actions-output.md) for
the design.
