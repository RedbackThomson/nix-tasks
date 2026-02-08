{
  description = "Company-wide nix-tasks standards - example template";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      # Support multiple systems
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Build standard config for a given system
      mkStandardConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          # ==========================================================
          # STANDARD PACKAGES
          # ==========================================================
          # These are the blessed versions of tools used across the organization.
          # All repositories should use these versions for consistency.
          packages = {
            # Go toolchain - pinned version
            go = pkgs.go;

            # Container tooling
            docker = pkgs.docker;

            # Kubernetes tools
            kubectl = pkgs.kubectl;
            helm = pkgs.kubernetes-helm;
            k9s = pkgs.k9s;

            # Code quality
            golangci-lint = pkgs.golangci-lint;
            shellcheck = pkgs.shellcheck;

            # Build tools
            gnumake = pkgs.gnumake;
            jq = pkgs.jq;
            yq = pkgs.yq;

            # Testing
            gotestsum = pkgs.gotestsum;
          };

          # ==========================================================
          # STANDARD TASKS
          # ==========================================================
          # These tasks provide consistent behavior across all repositories.
          # Repositories can override specific fields using compose helpers.
          tasks = {
            # Code quality tasks
            lint = {
              description = "Run standard linters (golangci-lint)";
              deps = [ "golangci-lint" ];
              commands = [
                "golangci-lint run --timeout 5m ./..."
              ];
              continueOnError = true;
            };

            lint-shell = {
              description = "Lint shell scripts with shellcheck";
              deps = [ "shellcheck" ];
              commands = [
                "find . -name '*.sh' -type f | xargs -r shellcheck"
              ];
              continueOnError = true;
            };

            fmt = {
              description = "Format Go code";
              deps = [ "go" ];
              commands = [
                "go fmt ./..."
              ];
            };

            fmt-check = {
              description = "Check Go code formatting (fails if unformatted)";
              deps = [ "go" ];
              commands = [
                "test -z \"$(gofmt -l . | tee /dev/stderr)\""
              ];
            };

            # Testing tasks
            test-unit = {
              description = "Run unit tests";
              deps = [ "go" "gotestsum" ];
              commands = [
                "gotestsum --format standard-verbose -- -race -coverprofile=coverage.out ./..."
              ];
              env = {
                CGO_ENABLED = "1";
              };
            };

            test-short = {
              description = "Run short tests only";
              deps = [ "go" ];
              commands = [
                "go test -short ./..."
              ];
            };

            # Build tasks
            build = {
              description = "Build Go binary";
              deps = [ "go" ];
              commands = [
                "go build -v ./..."
              ];
              inputs = [ "**/*.go" "go.mod" "go.sum" ];
            };

            # Dependency management
            deps-tidy = {
              description = "Tidy Go modules";
              deps = [ "go" ];
              commands = [
                "go mod tidy"
              ];
            };

            deps-verify = {
              description = "Verify Go module checksums";
              deps = [ "go" ];
              commands = [
                "go mod verify"
              ];
            };

            deps-download = {
              description = "Download Go dependencies";
              deps = [ "go" ];
              commands = [
                "go mod download"
              ];
            };

            # Security
            security-scan = {
              description = "Run security vulnerability scan";
              deps = [ "go" ];
              commands = [
                "go install golang.org/x/vuln/cmd/govulncheck@latest"
                "govulncheck ./..."
              ];
              continueOnError = true;
            };

            # Clean
            clean = {
              description = "Clean build artifacts";
              deps = [];
              commands = [
                "rm -rf bin/ dist/ coverage.out"
              ];
            };
          };

          # ==========================================================
          # STANDARD DEV SHELLS
          # ==========================================================
          # These shells provide consistent development environments.
          # Repositories can extend these with additional packages.
          devShells = {
            # Minimal shell - just Go
            minimal = {
              packages = [ "go" ];
              env = {
                CGO_ENABLED = "0";
              };
            };

            # CI shell - tools needed for CI pipelines
            ci = {
              extends = "minimal";
              packages = [ "golangci-lint" "gotestsum" "jq" ];
              env = {
                CI = "true";
              };
            };

            # Full development shell
            default = {
              extends = "ci";
              packages = [ "docker" "kubectl" "helm" "k9s" "shellcheck" "yq" ];
              shellHook = ''
                echo "Company Standard Development Environment"
                echo "========================================="
                echo "Go version: $(go version)"
                echo ""
                echo "Available tools: go, golangci-lint, docker, kubectl, helm, k9s"
                echo "Run 'nix-tasks list' to see available tasks"
                echo ""
              '';
            };
          };
        };
    in {
      # ==========================================================
      # EXPORTS
      # ==========================================================

      # Expose nix-tasks CLI as a package and app
      packages = forAllSystems (system: {
        nix-tasks = nix-tasks.packages.${system}.default;
        default = nix-tasks.packages.${system}.default;
      });

      apps = forAllSystems (system: {
        nix-tasks = nix-tasks.apps.${system}.default;
        default = nix-tasks.apps.${system}.default;
      });

      # Re-export nix-tasks lib with company extensions
      lib = forAllSystems (system:
        let
          baseLib = nix-tasks.lib.${system};
          pkgs = nixpkgs.legacyPackages.${system};
        in baseLib // {
          # Add company-specific builders here
          # mkServiceTask = ...;
          # mkLambdaTask = ...;
        }
      );

      # Export standard config per system for repositories to extend
      standardConfig = forAllSystems mkStandardConfig;

      # For testing: evaluated config with devShells that include nix-tasks
      devShells = forAllSystems (system:
        let
          lib = nix-tasks.lib.${system};
          config = lib.evalConfig (mkStandardConfig system);
        in
        builtins.mapAttrs (name: shell:
          shell.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          })
        ) config.devShells
      );

      # For testing: export the evaluated config
      nixTasksConfig = forAllSystems (system:
        let
          lib = nix-tasks.lib.${system};
          config = lib.evalConfig (mkStandardConfig system);
        in config.nixTasksConfig
      );

      nixTasksShells = forAllSystems (system:
        let
          lib = nix-tasks.lib.${system};
          config = lib.evalConfig (mkStandardConfig system);
        in config.nixTasksShells
      );
    };
}
