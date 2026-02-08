# Simple nix-tasks Example

This example demonstrates basic nix-tasks usage with simple build, test, and clean tasks.

## Usage

### Run tasks directly

```bash
# Run the build task
nix run .#nix-tasks -- run build

# Run the test task
nix run .#nix-tasks -- run test

# Run the clean task
nix run .#nix-tasks -- run clean

# List all available tasks
nix run .#nix-tasks -- list
```

### Use in development shell

```bash
# Enter the dev shell (includes nix-tasks)
nix develop

# Now nix-tasks is available directly
nix-tasks list
nix-tasks run build
nix-tasks run test
```

### Shorter syntax with default app

```bash
# The default app is nix-tasks, so you can use:
nix run . -- list
nix run . -- run build
nix run . -- run test
```
