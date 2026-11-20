### nix-tasks: deploy ✗

| Task | Result | Duration | Notes |
|------|--------|---------:|-------|
| generate | ✓ passed | 12.4s |  |
| generate-crossplane-provider-aws-ec2-managed-resources-crds | ✓ passed | 13.0s |  |
| lint | ✗ failed | 3.1s | exit 1 |
| vet | ● cached | — |  |
| fmt-check | ! failed (allowed) | 0.4s | exit 1 |
| build | ✓ passed | 41.0s |  |
| deploy | - skipped | — | lint failed |

**1 failed** · 1 allowed failure · 3 passed · 1 cached · 1 skipped · 56.2s total

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
