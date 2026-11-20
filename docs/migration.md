# Migrating to the module-based configuration

nix-tasks configuration is now evaluated with the Nix module system
(`lib.evalModules`). Tasks, packages and dev shells are typed options, shared
standards are modules a repository imports, and the merge markers and task
modifiers are replaced by the module system's own `mkForce`, `mkBefore`,
`mkAfter` and `mkDefault`.

The CLI only reads the new output shape (package names instead of store
paths), so update the flake's `nix-tasks` input and its configuration
together.

## Steps

1. Add `nix-tasks.inputs.nixpkgs.follows = "nixpkgs";` (and the same for any
   standards flake), so builders and task shells use your nixpkgs.
2. Replace the per-system `mkConfig`/`evalConfig` boilerplate and the
   `nixTasksConfig`/`nixTasksShells`/`nixTasksBuilds`/`devShells` outputs with
   `nix-tasks.lib.mkFlakeOutputs { inherit nixpkgs; modules = [ ... ]; }`.
3. Move the attribute set you passed to `evalConfig` into a module:
   `({ pkgs, lib, tasksLib, ... }: { packages = ...; tasks = ...; devShells = ...; })`.
4. Replace `lib.mkTask` and the other builders with `tasksLib.mkTask` etc.
   (or plain attribute sets; `mkTask` is the identity).
5. Turn each `extend`, marker and modifier into a definition in your module,
   using the table below.
6. For a standards flake, export the module (`nixTasksModules.default =
   ./standards.nix;`) instead of a `standardConfig` attribute set, and list it
   in the repository's `modules`.
7. Run `nix flake check` / `nix-tasks validate`. Typos, wrong types, unknown
   references and conflicting commands are now evaluation errors that name the
   option.

## Old API → new API

| Old | New |
|-----|-----|
| `nix-tasks.lib.${system}.evalConfig cfg` | `nix-tasks.lib.mkFlakeOutputs { inherit nixpkgs; modules = [ cfg ]; }` (or `nix-tasks.lib.evalConfig { inherit pkgs; modules = [ cfg ]; }` for one system) |
| `nixTasksConfig = forAllSystems (...)` etc. | Returned by `mkFlakeOutputs` (adds `nixTasksShellDrvs`) |
| `lib.mkTask`, `lib.mkShellTask`, `lib.mkGoTask`, `lib.mkDockerTask`, `lib.mkCompoundTask` | `tasksLib.<same>` (module argument), or `nix-tasks.lib.mkLib pkgs` |
| `lib.compat.make.*` | `tasksLib.compat.make.*` |
| `standardConfig.${system}` + `lib.extend standards { ... }` | `modules = [ standards.nixTasksModules.default ./repo.nix ]` |
| `lib.extend base (base: { ... })` | A module; read other values through its `config` argument |
| `lib.override value` | `lib.mkForce value` |
| `lib.append [ ... ]` on lists | A plain list (lists from all modules are concatenated); `lib.mkAfter` for commands/script |
| `lib.prepend [ ... ]` | `lib.mkBefore [ ... ]` |
| `lib.mergeAttrs a b` | Two modules |
| `prependTaskDeps names task` | `tasks.<t>.depends = lib.mkBefore (map (n: "task:${n}") names);` |
| `appendTaskDeps names task` | `tasks.<t>.depends = map (n: "task:${n}") names;` |
| `overrideTaskDeps names task` | `tasks.<t>.depends = lib.mkForce (...);` |
| `prependAfterHooks` / `appendAfterHooks` / `overrideAfterHooks` | `tasks.<t>.after = lib.mkBefore [...]` / `[...]` / `lib.mkForce [...]` |
| `prependDeps` / `appendDeps` / `overrideDeps` | `tasks.<t>.deps = lib.mkBefore [...]` / `[...]` / `lib.mkForce [...]` |
| `prependCommands cmds task` | `tasks.<t>.commands = lib.mkBefore cmds;` (on a `script` task: `tasks.<t>.script = lib.mkBefore "..."`) |
| `appendCommands cmds task` | `tasks.<t>.commands = lib.mkAfter cmds;` (or `script = lib.mkAfter "..."`) |
| `overrideCommands cmds task` | `tasks.<t>.commands = lib.mkForce cmds;` (and `script = lib.mkForce null;` if the base used `script`) |
| `mergeEnv vars task` | `tasks.<t>.env = vars;` |
| `overrideEnv vars task` | `tasks.<t>.env = lib.mkForce vars;` |
| `appendInputs` / `overrideInputs` | `tasks.<t>.inputs = [...]` / `lib.mkForce [...]` |
| `setDescription d` | `tasks.<t>.description = d;` (`lib.mkForce d` if the base sets a plain description) |
| `setWorkingDir` / `setNoCache` / `setContinueOnError` | Set the field (with `lib.mkForce` if the base sets a different plain value) |
| `pipe task [ ... ]` | One module defining each field |
| `mkGoTask { name = ...; }` | `pname` (required); `name` is gone |
| `mkGoTask { vendorSha256 = ...; }` | `vendorHash` |
| `mkGoTask { ldflags = "-s -w"; }` | `ldflags = [ "-s" "-w" ];` (default `[]`) |
| `mkGoTask { env = ...; }` adding to the task's env | `env` is set on the derivation only |
| `mkGoTask { workingDir = ...; }` | Remove it: build tasks reject `workingDir`, `commands`, `script` and `inputs` |
| `mkTask { name = ...; }`, `mkCompoundTask { name = ...; }` | Remove `name`; the attribute name is the task name |
| `devShells.<s>.env.shellHook = ...` (hijacked `mkShell` args) | `devShells.<s>.shellHook = ...`; `env` only sets environment variables |
| Repo dev shells adding the CLI via `overrideAttrs` | `packages.nix-tasks = nix-tasks.packages.${pkgs.stdenv.hostPlatform.system}.default;` and list `"nix-tasks"` in the shell's `packages` |
| `packages`/`apps` re-export boilerplate | `// { inherit (nix-tasks) packages apps; }` |

### Behaviour that changed

- **`commands` and `script` no longer merge implicitly.** Two modules setting
  them with plain values is an error; say `mkForce`, `mkBefore` or `mkAfter`.
  Identical definitions are allowed.
- **Lists concatenate.** A repository that defines `devShells.default.packages`
  adds to the standard's list instead of replacing it; use `lib.mkForce` to
  replace. `deps`, `depends`, `after`, `inputs` and shell `packages` are
  de-duplicated.
- **Scalar conflicts are errors.** If two modules give `description`,
  `continueOnError`, `workingDir` etc. different plain values, evaluation
  fails. Standards should use `lib.mkDefault` for values repositories are
  expected to change; repositories can use `lib.mkForce`.
- **`null` clears a value** (`lib.mkForce null`), and derivations in
  `packages` are replaced whole, never merged attribute by attribute.
- **Types are checked.** `env` values must be strings, and unknown task or
  shell fields (`outputs`, typos) are errors.
- **References are checked at evaluation time**: unknown `depends`/`after`
  tasks, unknown `deps` and shell packages, unknown `extends` targets, shell
  `extends` cycles (previously a stack overflow), and tasks that depend on or
  run after themselves. All problems are reported together.
- **`commands` plus `script` on one task is an error** (previously `script`
  silently won).
- **`nixTasksConfig.packages` is a list of names.** The cache fingerprint uses
  the task shell's `drvPath` (`shellDrvPath`, emitted only for cacheable
  tasks) instead of package store paths. Existing cache entries miss once.
- **Builders return partial definitions.** Their invented defaults
  (`mkGoTask`'s description, `mkDockerTask`'s command, `mkMakeTask`'s
  description) are `mkDefault`, so a builder in one module no longer resets
  fields set by another.
- **`nixTasksShells` contains only shell tasks**, and the new
  `nixTasksShellDrvs.<system>.<task>` exposes each task shell's `drvPath`.

## Full example: `examples/app-using-standards`

### Before

```nix
{
  description = "Example app using company standards with customizations";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
    # In a real scenario, this would be:
    # company-standards.url = "github:your-company/nix-tasks-standards";
    company-standards.url = "path:../company-standards";
  };

  outputs = { self, nixpkgs, nix-tasks, company-standards }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
          standards = company-standards.standardConfig.${system};

          # Use lib.extend to merge company standards with repo-specific config
          config = lib.extend standards {
            # ==========================================================
            # ADDITIONAL PACKAGES
            # ==========================================================
            # Add repo-specific packages to the standard set
            packages = {
              # Add Node.js for this specific project's frontend
              nodejs = pkgs.nodejs_20;
              # Add protobuf tooling for this gRPC service
              protobuf = pkgs.protobuf;
              protoc-gen-go = pkgs.protoc-gen-go;
              protoc-gen-go-grpc = pkgs.protoc-gen-go-grpc;
            };

            # ==========================================================
            # TASK CUSTOMIZATIONS
            # ==========================================================
            tasks = {
              # ------------------------------------------------------
              # Override the standard lint command with stricter settings
              # Using compose markers (config-level merge)
              # ------------------------------------------------------
              lint = {
                commands = lib.override [
                  "golangci-lint run --timeout 10m --config .golangci.yml ./..."
                ];
              };

              # ------------------------------------------------------
              # Modify standard test using task modifiers
              # These operate directly on the task attr set
              # ------------------------------------------------------
              test-unit = lib.prependCommands
                [ "echo 'Running app-specific test setup...'" ]
                standards.tasks.test-unit;

              # ------------------------------------------------------
              # Modify standard build using pipe for multiple changes
              # ------------------------------------------------------
              build = lib.pipe standards.tasks.build [
                (lib.overrideCommands [
                  "go build -ldflags '-s -w' -o bin/myservice ./cmd/myservice"
                ])
                (lib.mergeEnv { BUILD_VERSION = "1.0.0"; })
              ];

              # ------------------------------------------------------
              # Add project-specific tasks
              # ------------------------------------------------------
              proto-gen = lib.mkTask {
                description = "Generate Go code from protobuf definitions";
                deps = [ "protobuf" "protoc-gen-go" "protoc-gen-go-grpc" ];
                commands = [
                  "protoc --go_out=. --go_opt=paths=source_relative \\"
                  "       --go-grpc_out=. --go-grpc_opt=paths=source_relative \\"
                  "       api/v1/*.proto"
                ];
                inputs = [ "api/**/*.proto" ];
              };

              build-frontend = lib.mkTask {
                description = "Build frontend assets";
                deps = [ "nodejs" ];
                commands = [
                  "cd frontend && npm ci && npm run build"
                ];
                inputs = [ "frontend/**/*" ];
              };

              docker-build = lib.mkTask {
                description = "Build Docker image";
                deps = [];
                depends = [ "task:build" "task:build-frontend" ];
                commands = [
                  "docker build -t myservice:latest ."
                ];
              };

              deploy-staging = lib.mkTask {
                description = "Deploy to staging";
                deps = [ "kubectl" "helm" ];
                depends = [ "task:docker-build" "task:test-unit" ];
                commands = [
                  "helm upgrade --install myservice ./charts/myservice \\"
                  "  --namespace staging \\"
                  "  --set image.tag=latest"
                ];
                env = {
                  ENVIRONMENT = "staging";
                };
              };

              # ------------------------------------------------------
              # After-hooks: run automatically after a standard task
              # These let you hook into standard tasks without modifying
              # their definitions - ideal for extending shared configs.
              # ------------------------------------------------------
              generate-sbom = lib.mkTask {
                description = "Generate software bill of materials after build";
                after = [ "task:build" ];
                commands = [
                  "echo 'Generating SBOM from build artifacts...'"
                  "echo '{\"format\": \"cyclonedx\", \"components\": []}' > sbom.json"
                ];
              };

              # ------------------------------------------------------
              # Compound tasks
              # ------------------------------------------------------
              ci = lib.mkCompoundTask {
                description = "Full CI pipeline";
                tasks = [ "lint" "test-unit" "build" ];
              };

              # Extend a compound task: prepend proto-gen before everything
              all = lib.prependTaskDeps [ "proto-gen" ] (lib.mkCompoundTask {
                description = "Build everything";
                tasks = [ "build" "build-frontend" ];
              });

              # ------------------------------------------------------
              # Make compatibility - wrap legacy targets during migration
              # ------------------------------------------------------
            } // lib.compat.make.importMakeTargets {
              targets = [ "legacy-migrate" "legacy-seed" ];
              prefix = "db";
            };

            # ==========================================================
            # SHELL CUSTOMIZATIONS
            # ==========================================================
            devShells = {
              # Add project-specific packages to the default shell
              default = {
                # Append additional packages to what's inherited from ci
                packages = lib.append [ "nodejs" "protobuf" "protoc-gen-go" "protoc-gen-go-grpc" ];
                # Override the shell hook
                shellHook = lib.override ''
                  echo "MyService Development Environment"
                  echo "=================================="
                  echo "Go version: $(go version)"
                  echo "Node version: $(node --version)"
                  echo ""
                  echo "Project-specific tasks:"
                  echo "  nix-tasks run proto-gen      - Generate protobuf code"
                  echo "  nix-tasks run build-frontend - Build frontend"
                  echo "  nix-tasks run ci             - Run CI pipeline"
                  echo ""
                '';
              };

              # Add a frontend-only shell for frontend developers
              frontend = {
                packages = [ "nodejs" ];
                shellHook = ''
                  echo "Frontend Development Environment"
                  cd frontend 2>/dev/null || true
                '';
              };
            };
          };
        in lib.evalConfig config;
    in {
      # Expose nix-tasks CLI as a package and app
      packages = forAllSystems (system: {
        nix-tasks = nix-tasks.packages.${system}.default;
        default = nix-tasks.packages.${system}.default;
      });

      apps = forAllSystems (system: {
        nix-tasks = nix-tasks.apps.${system}.default;
        default = nix-tasks.apps.${system}.default;
      });

      # Expose task configuration for nix-tasks CLI
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);

      # Expose dev shells with nix-tasks available
      devShells = forAllSystems (system:
        let
          config = mkConfig system;
        in
        builtins.mapAttrs (name: shell:
          shell.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          })
        ) config.devShells
      );
    };
}
```

### After

```nix
{
  description = "Example app using company standards with customizations";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
    # In a real scenario, this would be:
    # company-standards.url = "github:your-company/nix-tasks-standards";
    company-standards.url = "path:../company-standards";
    company-standards.inputs.nixpkgs.follows = "nixpkgs";
    company-standards.inputs.nix-tasks.follows = "nix-tasks";
  };

  outputs = { self, nixpkgs, nix-tasks, company-standards }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        company-standards.nixTasksModules.default

        ({ lib, pkgs, tasksLib, ... }: {
          # Repo-specific packages, added to the standard set.
          packages = {
            nodejs = pkgs.nodejs_20;
            protobuf = pkgs.protobuf;
            protoc-gen-go = pkgs.protoc-gen-go;
            protoc-gen-go-grpc = pkgs.protoc-gen-go-grpc;
            nix-tasks = nix-tasks.packages.${pkgs.stdenv.hostPlatform.system}.default;
          };

          tasks = {
            # Replace the standard lint command with stricter settings.
            lint.commands = lib.mkForce [
              "golangci-lint run --timeout 10m --config .golangci.yml ./..."
            ];

            # Run a setup step before the standard test commands.
            test-unit.commands = lib.mkBefore [
              "echo 'Running app-specific test setup...'"
            ];

            build = {
              commands = lib.mkForce [
                "go build -ldflags '-s -w' -o bin/myservice ./cmd/myservice"
              ];
              env.BUILD_VERSION = "1.0.0";
            };

            proto-gen = tasksLib.mkTask {
              description = "Generate Go code from protobuf definitions";
              deps = [ "protobuf" "protoc-gen-go" "protoc-gen-go-grpc" ];
              commands = [
                "protoc --go_out=. --go_opt=paths=source_relative \\"
                "       --go-grpc_out=. --go-grpc_opt=paths=source_relative \\"
                "       api/v1/*.proto"
              ];
              inputs = [ "api/**/*.proto" ];
            };

            build-frontend = tasksLib.mkTask {
              description = "Build frontend assets";
              deps = [ "nodejs" ];
              commands = [
                "cd frontend && npm ci && npm run build"
              ];
              inputs = [ "frontend/**/*" ];
            };

            docker-build = tasksLib.mkTask {
              description = "Build Docker image";
              depends = [ "task:build" "task:build-frontend" ];
              commands = [
                "docker build -t myservice:latest ."
              ];
            };

            deploy-staging = tasksLib.mkTask {
              description = "Deploy to staging";
              deps = [ "kubectl" "helm" ];
              depends = [ "task:docker-build" "task:test-unit" ];
              commands = [
                "helm upgrade --install myservice ./charts/myservice \\"
                "  --namespace staging \\"
                "  --set image.tag=latest"
              ];
              env = {
                ENVIRONMENT = "staging";
              };
            };

            # After-hooks run automatically after a standard task, without
            # touching the standard definition.
            generate-sbom = tasksLib.mkTask {
              description = "Generate software bill of materials after build";
              after = [ "task:build" ];
              commands = [
                "echo 'Generating SBOM from build artifacts...'"
                "echo '{\"format\": \"cyclonedx\", \"components\": []}' > sbom.json"
              ];
            };

            ci = tasksLib.mkCompoundTask {
              description = "Full CI pipeline";
              tasks = [ "lint" "test-unit" "build" ];
            };

            all = tasksLib.mkCompoundTask {
              description = "Build everything";
              tasks = [ "proto-gen" "build" "build-frontend" ];
            };
          } // tasksLib.compat.make.importMakeTargets {
            # Legacy Make targets, wrapped during migration.
            targets = [ "legacy-migrate" "legacy-seed" ];
            prefix = "db";
          };

          devShells = {
            # Appended to the packages the standard default shell declares.
            default = {
              packages = [ "nodejs" "protobuf" "protoc-gen-go" "protoc-gen-go-grpc" "nix-tasks" ];
              shellHook = lib.mkForce ''
                echo "MyService Development Environment"
                echo "=================================="
                echo "Go version: $(go version)"
                echo "Node version: $(node --version)"
                echo ""
                echo "Project-specific tasks:"
                echo "  nix-tasks run proto-gen      - Generate protobuf code"
                echo "  nix-tasks run build-frontend - Build frontend"
                echo "  nix-tasks run ci             - Run CI pipeline"
                echo ""
              '';
            };

            frontend = {
              packages = [ "nodejs" "nix-tasks" ];
              shellHook = ''
                echo "Frontend Development Environment"
                cd frontend 2>/dev/null || true
              '';
            };
          };
        })
      ];
    } // {
      inherit (nix-tasks) packages apps;
    };
}
```

The standards flake it imports changes the same way: the attribute set returned by `mkStandardConfig` becomes [`examples/company-standards/standards.nix`](../examples/company-standards/standards.nix), exported as `nixTasksModules.default`.
