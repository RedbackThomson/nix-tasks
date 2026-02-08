# Company Standards Example

This example demonstrates how to create a company-wide nix-tasks standards flake that can be shared across all repositories in an organization.

## Purpose

The company-standards flake provides:

1. **Standard package versions** - Blessed versions of tools (Go, Docker, kubectl, etc.) used across the organization
2. **Standard tasks** - Common tasks (lint, test, build) with consistent behavior
3. **Standard dev shells** - Base development environments that repos can extend
4. **Extensibility** - A `standardConfig` export that repos can import and customize

## Usage

### For Repository Owners

Repositories import this flake and extend it with repo-specific customizations. See the [app-using-standards](../app-using-standards/) example for a complete demonstration.

```nix
{
  inputs = {
    company-standards.url = "github:your-company/nix-tasks-standards";
  };

  outputs = { company-standards, nix-tasks, ... }:
    let
      lib = nix-tasks.lib.${system};
      standards = company-standards.standardConfig.${system};

      # Extend standards with repo-specific config
      config = lib.extend standards {
        # Add repo-specific packages, tasks, and shells
      };
    in lib.evalConfig config;
}
```

### For Standards Maintainers

This flake itself can be used to test the standard configuration:

```bash
# Test the standard dev shells
nix develop                    # default shell
nix develop .#ci              # CI shell
nix develop .#minimal         # minimal shell

# Run standard tasks
nix run . -- list
nix run . -- run lint
nix run . -- run test-unit
```

## Standard Packages

The following tool versions are standardized across the organization:

- **go** - Go toolchain (pinned version)
- **docker** - Docker CLI
- **kubectl** - Kubernetes CLI
- **helm** - Helm package manager
- **k9s** - Kubernetes TUI
- **golangci-lint** - Go linter
- **shellcheck** - Shell script linter
- **gotestsum** - Go test runner with better output
- **jq** / **yq** - JSON/YAML processors

## Standard Tasks

All repositories inherit these standard tasks:

### Code Quality

- `lint` - Run golangci-lint with standard configuration
- `lint-shell` - Lint shell scripts with shellcheck
- `fmt` - Format Go code
- `fmt-check` - Check Go code formatting (fails if unformatted)

### Testing

- `test-unit` - Run unit tests with coverage
- `test-short` - Run short tests only

### Build

- `build` - Build Go binary

### Dependencies

- `deps-tidy` - Tidy Go modules
- `deps-verify` - Verify Go module checksums
- `deps-download` - Download Go dependencies

### Security

- `security-scan` - Run vulnerability scan with govulncheck

### Utility

- `clean` - Clean build artifacts

## Standard Dev Shells

Three development shells with inheritance:

```text
minimal (just Go)
    
    �
   ci (+ linters and test tools)
    
    �
default (+ all Kubernetes tools)
```

### Shell Usage

```bash
# Enter minimal shell (just Go)
nix develop .#minimal

# Enter CI shell (Go + CI tools)
nix develop .#ci

# Enter full development shell
nix develop
```

## Customization Guidelines

Repositories extending these standards should:

1. **Use `lib.extend`** to merge with standards
2. **Use override markers** for explicit modifications:
   - `lib.override` - Replace a value completely
   - `lib.append` - Add to the end of a list
   - `lib.prepend` - Add to the beginning of a list
3. **Add repo-specific tasks** - New tasks unique to the repository
4. **Extend shells** - Add packages to standard shells without replacing them

See [app-using-standards](../app-using-standards/) for examples.

## Updating Standards

When updating tool versions or standard configurations:

1. Update this flake
2. Test with `nix flake check`
3. Tag a new release
4. Repositories update their `company-standards` input and run `nix flake update`

## Company-Specific Builders

You can add company-specific task builders by extending the `lib` export:

```nix
lib = forAllSystems (system:
  let
    baseLib = nix-tasks.lib.${system};
  in baseLib // {
    # Add custom builder
    mkServiceTask = { name, ... }: {
      # Custom task configuration
    };
  }
);
```
