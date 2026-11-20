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
