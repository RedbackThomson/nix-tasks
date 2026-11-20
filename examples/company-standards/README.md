# Company Standards Example

This example demonstrates how to create a company-wide nix-tasks standards flake that can be shared across all repositories in an organization.

## Purpose

The company-standards flake provides:

1. **Standard package versions** - Blessed versions of tools (Go, Docker, kubectl, etc.) used across the organization
2. **Standard tasks** - Common tasks (lint, test, build) with consistent behavior
3. **Standard dev shells** - Base development environments that repos can extend
4. **Extensibility** - A `nixTasksModules.default` module that repos import and customize

## Usage

### For Repository Owners

Repositories import the standards module and add their own module next to it. See the [app-using-standards](../app-using-standards/) example for a complete demonstration.

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
    company-standards.url = "github:your-company/nix-tasks-standards";
    company-standards.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { nixpkgs, nix-tasks, company-standards, ... }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        company-standards.nixTasksModules.default
        ./nix-tasks.nix   # repo-specific packages, tasks and shells
      ];
    };
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

1. **Import the module** - add `company-standards.nixTasksModules.default` to `modules`
2. **Say how a value changes** when the standard already sets it:
   - `lib.mkForce` - replace a value completely
   - `lib.mkBefore` / `lib.mkAfter` - add commands, depends or packages before / after the standard ones
   - plain definitions of lists such as `deps`, `depends` or shell `packages` are appended
   - `commands` and `script` refuse to merge two plain definitions, so a repo cannot accidentally run both its own and the standard commands
3. **Add repo-specific tasks** - new tasks unique to the repository
4. **Extend shells** - define the same shell again to add packages, env or shellHook lines

Standards mark values that repos are expected to change with `lib.mkDefault` (for example `continueOnError` on the lint tasks), so a plain repo definition replaces them without `mkForce`.

See [app-using-standards](../app-using-standards/) for examples.

## Updating Standards

When updating tool versions or standard configurations:

1. Update this flake
2. Test with `nix flake check`
3. Tag a new release
4. Repositories update their `company-standards` input and run `nix flake update`

## Company-Specific Builders

Builders are ordinary functions that return task definitions, so a standards flake can export its own next to the module:

```nix
lib.mkServiceTask = { service, ... }: {
  description = "Build ${service}";
  deps = [ "go" ];
  commands = [ "go build -o bin/${service} ./cmd/${service}" ];
};
```
