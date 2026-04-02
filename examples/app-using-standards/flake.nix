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
                outputs = [ "api/**/*.pb.go" ];
              };

              build-frontend = lib.mkTask {
                description = "Build frontend assets";
                deps = [ "nodejs" ];
                commands = [
                  "cd frontend && npm ci && npm run build"
                ];
                inputs = [ "frontend/**/*" ];
                outputs = [ "frontend/dist/" ];
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
