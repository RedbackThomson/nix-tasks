{
  description = "nix-tasks demo - a mock Go web service";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      # Support multiple systems
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Build config for a given system
      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
        in lib.evalConfig {
          # Available packages for tasks and shells
          packages = {
            go = pkgs.go;
            nodejs = pkgs.nodejs;
            jq = pkgs.jq;
            curl = pkgs.curl;
            gnused = pkgs.gnused;
          };

          tasks = {
            # ============ Code Generation ============
            generate = lib.mkTask {
              description = "Generate mocks and code";
              deps = [ "go" ];
              commands = [
                "echo 'Generating mocks...'"
                "mkdir -p internal/mocks"
                "echo '// Generated mock file' > internal/mocks/mocks.go"
                "echo 'Generation complete'"
              ];
            };

            # ============ Build Tasks ============
            build = lib.mkTask {
              description = "Build the application";
              deps = [ "go" ];
              depends = [ "task:generate" ];
              commands = [
                "echo 'Building application...'"
                "mkdir -p bin"
                "echo '#!/bin/sh' > bin/app"
                "echo 'echo Hello from app' >> bin/app"
                "chmod +x bin/app"
                "echo 'Build complete: bin/app'"
              ];
              inputs = [ "**/*.go" "go.mod" ];
              outputs = [ "bin/app" ];
            };

            build-frontend = lib.mkTask {
              description = "Build frontend assets";
              deps = [ "nodejs" ];
              commands = [
                "echo 'Building frontend...'"
                "mkdir -p dist"
                "echo '<html><body>Hello</body></html>' > dist/index.html"
                "echo 'Frontend build complete'"
              ];
              inputs = [ "frontend/**/*" ];
              outputs = [ "dist/" ];
            };

            # ============ Quality Tasks ============
            lint = lib.mkTask {
              description = "Run linters";
              deps = [ "go" ];
              depends = [ "task:generate" ];
              commands = [
                "echo 'Running linters...'"
                "echo 'Lint passed (mock)'"
              ];
              continueOnError = true;
            };

            fmt = lib.mkTask {
              description = "Format code";
              deps = [ "go" ];
              commands = [
                "echo 'Formatting code...'"
                "echo 'Format complete'"
              ];
            };

            # ============ Test Tasks ============
            test-unit = lib.mkTask {
              description = "Run unit tests";
              deps = [ "go" ];
              depends = [ "task:generate" ];
              commands = [
                "echo 'Running unit tests...'"
                "echo 'PASS: TestUserService'"
                "echo 'PASS: TestOrderService'"
                "echo 'All unit tests passed'"
              ];
            };

            test-integration = lib.mkTask {
              description = "Run integration tests";
              deps = [ "go" ];
              depends = [ "task:build" ];
              commands = [
                "echo 'Running integration tests...'"
                "echo 'PASS: TestAPIEndpoints'"
                "echo 'PASS: TestDatabaseConnection'"
                "echo 'All integration tests passed'"
              ];
            };

            test = lib.mkCompoundTask {
              description = "Run all tests";
              tasks = [ "test-unit" "test-integration" ];
            };

            # ============ Docker Tasks (mocked) ============
            docker-build = lib.mkTask {
              description = "Build Docker image";
              deps = [];
              depends = [ "task:build" "task:build-frontend" ];
              commands = [
                "echo 'Building Docker image...'"
                "echo 'Step 1/5: FROM golang:1.22-alpine'"
                "echo 'Step 2/5: COPY bin/app /app'"
                "echo 'Step 3/5: COPY dist/ /static/'"
                "echo 'Step 4/5: EXPOSE 8080'"
                "echo 'Step 5/5: CMD [\"/app\"]'"
                "echo 'Successfully built myapp:latest'"
              ];
            };

            docker-push = lib.mkTask {
              description = "Push Docker image to registry";
              deps = [];
              depends = [ "task:docker-build" ];
              commands = [
                "echo 'Pushing image to registry...'"
                "echo 'myapp:latest -> registry.example.com/myapp:latest'"
                "echo 'Push complete'"
              ];
            };

            # ============ Deployment Tasks ============
            deploy-staging = lib.mkTask {
              description = "Deploy to staging environment";
              deps = [ "jq" ];
              depends = [ "task:docker-push" "task:test" ];
              commands = [
                "echo 'Deploying to staging...'"
                "echo 'Applying k8s manifests to staging namespace'"
                "echo 'Waiting for rollout...'"
                "echo 'Deployment to staging complete'"
              ];
              env = {
                ENVIRONMENT = "staging";
                REPLICAS = "2";
              };
            };

            deploy-prod = lib.mkTask {
              description = "Deploy to production environment";
              deps = [ "jq" ];
              depends = [ "task:deploy-staging" ];
              commands = [
                "echo 'Deploying to production...'"
                "echo 'Applying k8s manifests to production namespace'"
                "echo 'Waiting for rollout...'"
                "echo 'Deployment to production complete'"
              ];
              env = {
                ENVIRONMENT = "production";
                REPLICAS = "5";
              };
            };

            # ============ Utility Tasks ============
            clean = lib.mkTask {
              description = "Clean build artifacts";
              deps = [];
              commands = [
                "echo 'Cleaning build artifacts...'"
                "rm -rf bin/ dist/ internal/mocks/"
                "echo 'Clean complete'"
              ];
            };

            health-check = lib.mkTask {
              description = "Check service health";
              deps = [ "curl" "jq" ];
              commands = [
                "echo 'Checking service health...'"
                "echo '{\"status\": \"healthy\", \"version\": \"1.0.0\"}' | jq ."
              ];
            };

            # ============ Compound Tasks ============
            ci = lib.mkCompoundTask {
              description = "Run full CI pipeline";
              tasks = [ "lint" "test" "docker-build" ];
            };

            release = lib.mkCompoundTask {
              description = "Full release pipeline";
              tasks = [ "ci" "docker-push" "deploy-staging" ];
            };
          };

          devShells = {
            # Minimal shell for CI
            minimal = {
              packages = [ "go" ];
              env = {
                CGO_ENABLED = "0";
              };
            };

            # CI shell extends minimal
            ci = {
              extends = "minimal";
              packages = [ "jq" ];
              env = {
                CI = "true";
              };
            };

            # Full dev shell extends CI
            default = {
              extends = "ci";
              packages = [ "nodejs" "curl" ];
              shellHook = ''
                echo "Welcome to the nix-tasks demo project!"
                echo ""
                echo "Try: nix-tasks list"
                echo ""
              '';
            };
          };
        };
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
          pkgs = nixpkgs.legacyPackages.${system};
        in
        config.devShells // {
          # Extend all shells to include nix-tasks
          minimal = config.devShells.minimal.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          });
          ci = config.devShells.ci.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          });
          default = config.devShells.default.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          });
        }
      );
    };
}
