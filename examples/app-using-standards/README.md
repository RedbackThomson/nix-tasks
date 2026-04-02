# App Using Standards Example

This example demonstrates how a repository extends company-wide standards with project-specific customizations.

## Overview

This example shows:

1. **Importing company standards** - Uses `company-standards` flake as an input
2. **Extending with `lib.extend`** - Merges standards with repo-specific config
3. **Adding project packages** - Node.js, protobuf tools for this specific service
4. **Customizing standard tasks** - Override lint config, add test setup
5. **Adding new tasks** - Proto generation, frontend build, Docker, deployment
6. **After-hooks** - Hook onto standard tasks without modifying them
7. **Extending dev shells** - Add packages to standard shells
8. **Make compatibility** - Import legacy Make targets during migration

## Usage

### Run tasks directly

```bash
# List all available tasks (standard + custom)
nix run .#nix-tasks -- list

# Run standard tasks
nix run .#nix-tasks -- run lint
nix run .#nix-tasks -- run test-unit

# Run project-specific tasks
nix run .#nix-tasks -- run proto-gen
nix run .#nix-tasks -- run build-frontend
nix run .#nix-tasks -- run docker-build

# Run compound tasks
nix run .#nix-tasks -- run ci
nix run .#nix-tasks -- run all
```

### Use in development shell

```bash
# Enter the default dev shell (includes nix-tasks)
nix develop

# All tools and nix-tasks are now available
nix-tasks list
nix-tasks run build
nix-tasks run proto-gen

# Enter the frontend-only shell
nix develop .#frontend
```

### Shorter syntax with default app

```bash
# The default app is nix-tasks
nix run . -- list
nix run . -- run ci
nix run . -- run deploy-staging
```

## Configuration Structure

### Packages

Standard packages from company-standards, plus:

- `nodejs` - Node.js for frontend builds
- `protobuf` - Protobuf compiler
- `protoc-gen-go` - Go protobuf plugin
- `protoc-gen-go-grpc` - Go gRPC plugin

### Tasks

#### Inherited from Standards

These tasks come from `company-standards` and work out of the box:

- `lint` - (customized with stricter settings)
- `test-unit` - (extended with setup step)
- `build` - (overridden with custom binary path)
- `fmt`, `fmt-check` - Code formatting
- `deps-tidy`, `deps-verify`, `deps-download` - Dependency management
- `security-scan` - Vulnerability scanning
- `clean` - Clean build artifacts

#### Project-Specific Tasks

New tasks added for this service:

- `proto-gen` - Generate Go code from protobuf definitions
- `build-frontend` - Build frontend assets with npm
- `docker-build` - Build Docker image
- `deploy-staging` - Deploy to staging environment

#### After-Hooks

These tasks run automatically whenever their target task runs, without modifying the target:

- `generate-sbom` - Generates a software bill of materials (runs after `build`)

#### Compound Tasks

- `ci` - Full CI pipeline (lint + test-unit + build)
- `all` - Build everything (proto-gen + build + build-frontend)

#### Legacy Make Compatibility

During migration, this example wraps legacy Make targets:

- `db-legacy-migrate` - Wraps `make migrate`
- `db-legacy-seed` - Wraps `make seed`

### Development Shells

All shells inherit from `company-standards` and include nix-tasks:

- `default` - Full dev shell with Node.js and protobuf tools
- `frontend` - Frontend-only shell with just Node.js

## Customization Techniques

### 1. Override a Standard Task

Replace the entire command list:

```nix
tasks.lint = {
  commands = lib.override [
    "golangci-lint run --timeout 10m --config .golangci.yml ./..."
  ];
};
```

### 2. Extend a Standard Task

Add commands before existing ones:

```nix
tasks.test-unit = {
  commands = lib.prepend [
    "echo 'Running app-specific test setup...'"
  ];
};
```

### 3. Add New Tasks

Define completely new tasks:

```nix
tasks.proto-gen = lib.mkTask {
  description = "Generate Go code from protobuf definitions";
  deps = [ "protobuf" "protoc-gen-go" "protoc-gen-go-grpc" ];
  commands = [ "protoc --go_out=. ..." ];
};
```

### 4. Extend Dev Shells

Add packages to standard shells:

```nix
devShells.default = {
  packages = lib.append [ "nodejs" "protobuf" ];
  shellHook = lib.override ''
    echo "MyService Development Environment"
  '';
};
```

### 5. Hook onto Standard Tasks with After

Run a task automatically whenever a standard task runs, without modifying the standard task's definition:

```nix
tasks.generate-sbom = lib.mkTask {
  description = "Generate SBOM after build";
  after = [ "task:build" ];  # Runs whenever "build" runs
  commands = [ "generate-sbom > sbom.json" ];
};
```

When you run `nix-tasks run build`, both `build` and `generate-sbom` will execute in order. This is ideal for extending shared tasks since you don't need to touch the original task definition.

### 6. Create New Shells

Define project-specific shells:

```nix
devShells.frontend = {
  packages = [ "nodejs" ];
  shellHook = ''
    cd frontend 2>/dev/null || true
  '';
};
```

## How It Works

The key pattern is using `lib.extend`:

```nix
let
  standards = company-standards.standardConfig.${system};
  config = lib.extend standards {
    # Your customizations here
  };
in lib.evalConfig config;
```

This deep merges your config with company standards:

- New keys are added
- Existing keys with override markers (`lib.override`, `lib.append`, etc.) are modified
- Other existing keys are merged recursively

## Migration from Make

This example shows how to gradually migrate from Make to nix-tasks:

1. Keep your Makefile during migration
2. Use `lib.compat.make.importMakeTargets` to wrap legacy targets
3. Gradually rewrite Make targets as native nix-tasks
4. Remove Makefile when migration is complete

See the `db-legacy-*` tasks in [flake.nix](flake.nix) for an example.

## Development Workflow

Typical workflow for developers:

```bash
# Enter dev shell
nix develop

# Generate protobuf code
nix-tasks run proto-gen

# Build everything
nix-tasks run all

# Run tests
nix-tasks run test-unit

# Local development iteration
nix-tasks run build
./bin/myservice

# Deploy to staging
nix-tasks run deploy-staging
```

## CI/CD Workflow

In CI, you can run tasks without entering a shell:

```yaml
# .github/workflows/ci.yml
- name: Run CI pipeline
  run: nix run . -- run ci

- name: Deploy to staging
  run: nix run . -- run deploy-staging
  if: github.ref == 'refs/heads/main'
```

## Updating Standards

When company-standards releases a new version:

```bash
# Update the flake input
nix flake update company-standards

# Test that everything still works
nix-tasks run ci

# Commit the updated flake.lock
git add flake.lock
git commit -m "Update company standards"
```

Your customizations are preserved through the update.
