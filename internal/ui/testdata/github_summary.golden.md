### nix-tasks: deploy ✗

| Task | Result | Duration | Notes |
|------|--------|---------:|-------|
| generate | ✓ passed | 12.4s | not cached: depends on uncached tools |
| generate-crossplane-provider-aws-ec2-managed-resources-crds | ✓ passed | 13.0s |  |
| lint | ✗ failed | 3.1s | exit 1 |
| vet | ● cached | — | saved 18.2s |
| fmt-check | ! failed (allowed) | 0.4s | exit 1 |
| build | ✓ passed | 41.0s | not cached: inputs changed while it ran: go.sum |
| deploy | - skipped | — | lint failed |

**1 failed** · 1 allowed failure · 3 passed · 1 cached (saved 18.2s) · 1 skipped · 56.2s total

- ⚠️ generate has cache enabled but always runs: it depends on tools, which is not cached

<details><summary>✗ lint output</summary>

```text
apis/v1/types.go:42:2: ineffectual assignment to err (ineffassign)
::endgroup::
```

</details>

<details><summary>! fmt-check output</summary>

```text
main.go
```

</details>
