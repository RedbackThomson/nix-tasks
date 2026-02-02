# Nix-Based Task Runner - Requirements Summary

## Project Overview

**Goal:** Build a Nix-based task runner and development environment manager to replace Makefiles company-wide, providing:
- Standard tooling that can be imported and customized per repository
- Reproducible development environments with version consistency
- Clear, readable configuration compared to complex Make templates
- Company-wide standards distribution via Nix flakes

## Core Architecture Decisions

### 1. System Design Philosophy
**Unified Configuration Layer**
- Tasks and environments are equal first-class concepts that compose together
- Neither is "primary" - they're configuration objects managed by a composition system
- Tasks define their own dependencies separately from dev shells
- Version consistency enforced: tasks and shells must use same package versions

### 2. Configuration Composition
**Deep Merge with Override Markers**
- Child repos deep-merge with parent configs by default
- Explicit syntax for override semantics: `override`, `append`, `prepend`
- Intuitive defaults with clear vocabulary for modifications
- Enables company standards + repo-specific customization

### 3. Dependency Version Management
**Shared Package Registry**
- Central registry declares all package versions
- Both tasks and dev shells reference packages by name from registry
- Single source of truth for versions
- Tasks auto-include dependencies; shells are explicit collections

**Example:**
```nix
packages.go = pkgs.go_1_21;
packages.docker = pkgs.docker;

tasks.build.deps = ["go" "docker"];
devShells.default.packages = ["go" "docker" "kubectl"];
```

### 4. Task Definition Model
**Hybrid: Declarative with Shell Escape Hatch**
- Declare tasks in structured format (commands, dependencies, metadata)
- System generates executable shell script with setup/teardown
- Raw shell available for complex cases
- Generated scripts are inspectable

### 5. Task Dependencies
**Explicit DAG**
- Tasks declare dependencies on other tasks
- System builds dependency graph, executes in topological order
- Parallelizable execution
- Simple model: `depends = ["task:build" "task:test"]`

### 6. User Interface
**Multi-Modal Interface**
- **CLI**: Self-documenting with rich introspection
  - Commands: `list`, `describe`, `run`
  - Tasks have built-in descriptions, usage docs
- **TUI**: Interactive menu for exploration
  - Browse tasks, shells, configuration
  - Shows dependencies and recent history
- **Shell Integration**: Tasks as commands in dev shells
  - Tab completion
  - Natural workflow when in dev environment

### 7. Error Handling
**Configurable Failure Strategy**
- Per-task or per-run configuration:
  - Fail-fast (default)
  - Continue-on-error
  - Best-effort
- Complete independent DAG branches even if one fails
- Summary shows all successes/failures

### 8. Company Standards Distribution
**Flake Inputs with Version Pinning**
- Company standards published as Nix flake
- Each repo adds as flake input with specific version/commit
- Standard `nix flake update` workflow
- Explicit version control per repository

**Example:**
```nix
inputs = {
  company-tasks.url = "github:mycompany/nix-tasks-standard/v2.3.0";
};

outputs = { company-tasks, ... }: {
  tasks = company-tasks.lib.extend {
    # repo-specific overrides
  };
};
```

### 9. Caching Strategy
**Hybrid: Nix Store + Task-Level Caching**
- Environment/dependencies cached via Nix store
- Task execution results cached by fingerprint
- Binary cache (Cachix, S3) for team sharing
- Force rebuild flag to bypass cache for debugging

### 10. Environment Management
**Composition/Inheritance**
- Define base environments and compose them
- CI extends minimal, dev extends CI with extra tools
- Tasks reference minimum required environment
- Reduces duplication, clear relationships

**Example:**
```nix
devShells.minimal = { packages = ["go"]; };
devShells.ci = { extends = "minimal"; packages = ["docker"]; };
devShells.default = { extends = "ci"; packages = ["kubectl" "jq"]; };
```

### 11. Configuration Language
**Nix with Builder Helpers**
- Configuration in `.nix` files
- Rich helper library abstracts complexity
- Builders for common patterns: `mkGoTask`, `mkDockerTask`, `mkCompoundTask`
- Nix power with convenience layer

### 12. Migration Strategy
**Gradual with Make Compatibility**
- Support running Make targets: `nix-tasks run make:build`
- Migrate incrementally: hybrid state during transition
- Eventually deprecate Make when fully migrated
- Low friction, teams learn as they go

### 13. Output and Observability
**Context-Aware Output**
- **CI**: Streaming output with task prefixes
  - Real-time visibility
  - Interleaved logs show parallel execution
- **Local Dev**: Buffered with structured display
  - Clean output, hide success details
  - Failed tasks show full output
  - Reduced noise during development
- Auto-detect context or allow override: `--stream`

### 14. Cross-Repository Coordination
**Out of Scope**
- Focus on single-repository use cases
- Repos are independent
- Can be addressed in future iterations if needed

## Key Features

### Developer Experience
- **Discoverability**: `nix-tasks list`, `nix-tasks describe <task>`
- **Interactive TUI**: Menu-driven task exploration
- **Shell Integration**: Tasks available as commands in `nix develop`
- **Tab Completion**: Rich completion for tasks and options
- **Clear Error Messages**: Context-aware output based on environment

### Build System Features
- **Dependency Graph**: Automatic parallelization based on DAG
- **Caching**: Two-layer caching (Nix store + task fingerprints)
- **Reproducibility**: Version-locked dependencies via Nix
- **Flexibility**: Configurable failure strategies per task/run

### Company-Wide Standards
- **Distribution**: Via Nix flake inputs with version pinning
- **Customization**: Deep merge with override markers
- **Migration**: Gradual adoption with Make compatibility layer
- **Consistency**: Shared package registry ensures version alignment

## Implementation Considerations

### Core Components
1. **Task Runner**: Execute tasks in dependency order with caching
2. **Environment Manager**: Build and activate dev shells
3. **Configuration System**: Parse Nix configs, apply composition rules
4. **Cache Layer**: Integrate Nix store + custom task cache
5. **CLI/TUI**: User interfaces for interaction
6. **Builder Library**: Helper functions for common task patterns

### Technical Stack
- **Language**: Likely Rust or Go for CLI/runner performance
- **Nix Integration**: Heavy use of Nix evaluation and store
- **Caching Backend**: Local + remote (Cachix/S3)
- **TUI Framework**: bubbletea (Go) or ratatui (Rust)

### Success Criteria
- Faster than Make for incremental builds (via caching)
- More readable configuration than Make templates
- Successful adoption across multiple repos
- Reduced onboarding time for new developers
- Clear migration path preserves existing workflows

## Exploration Process

This requirements document was developed through systematic exploration of 14 key decision areas:

1. Core system responsibility and architecture
2. Configuration composition and override semantics
3. Dependency version management approach
4. Task definition and execution model
5. Task dependencies and orchestration
6. User interface and discoverability
7. Error handling and failure strategies
8. Company standards distribution mechanism
9. Caching and performance optimization
10. Multi-environment support (dev shells, CI)
11. Configuration syntax and validation
12. Migration path from existing Make infrastructure
13. Task output and observability
14. Cross-repository coordination

Each area was explored through structured options considering tradeoffs, similar systems, and practical implications for your company's workflow.
