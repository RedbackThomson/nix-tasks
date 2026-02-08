{
  description = "Nix-based task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = fn: nixpkgs.lib.genAttrs systems (system: fn system);

      mkTasksConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = self.lib.${system};
        in lib.evalConfig {
          packages = {
            go = pkgs.go;
            golangci-lint = pkgs.golangci-lint;
          };

          tasks = {
            # Build using Nix's buildGoModule for reproducibility
            build = lib.mkGoTask {
              pname = "nix-tasks";
              version = "0.1.0";
              description = "Build nix-tasks binary";
              src = ./.;
              vendorHash = "sha256-48Va8grh1BHGxZgwHKWvsB+HvBmmxD78cEDWl1Ysn4Y=";
              CGO_ENABLED = "0";
              ldflags = [ "-s" "-w" ];
            };

            # Run Go tests
            test = lib.mkTask {
              description = "Run Go tests";
              noCache = true;
              deps = [ "go" ];
              commands = [
                "go test -v ./..."
              ];
            };

            # Run linter
            lint = lib.mkTask {
              description = "Run golangci-lint";
              noCache = true;
              deps = [ "golangci-lint" ];
              commands = [
                "golangci-lint run ./..."
              ];
            };

            # Format code
            fmt = lib.mkTask {
              description = "Format Go code";
              noCache = true;
              deps = [ "go" ];
              commands = [
                "go fmt ./..."
              ];
            };

            # Tidy dependencies
            tidy = lib.mkTask {
              description = "Tidy Go dependencies";
              noCache = true;
              deps = [ "go" ];
              commands = [
                "go mod tidy"
              ];
            };

            # Clean build artifacts
            clean = lib.mkTask {
              description = "Clean build artifacts";
              commands = [
                "rm -rf result nix-tasks"
                "echo 'Cleaned build artifacts'"
              ];
            };

            # Run all checks (test + lint)
            check = lib.mkCompoundTask {
              description = "Run all checks";
              tasks = [ "test" "lint" ];
            };
          };

          devShells = {
            default = {
              packages = [ "go" "golangci-lint" ];
              shellHook = ''
                echo "nix-tasks development shell"
                echo "Available commands: go, golangci-lint"
              '';
            };
          };
        };
    in
    {
      packages = forAllSystems (system:
        let
          tasksConfig = mkTasksConfig system;
          # Use the build task's derivation directly
          nix-tasks = tasksConfig.rawTasks.build.derivation;
        in
        {
          default = nix-tasks;
          nix-tasks = nix-tasks;
        }
      );

      apps = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          nix-tasks = self.packages.${system}.nix-tasks;
        in
        {
          default = {
            type = "app";
            program = "${nix-tasks}/bin/nix-tasks";
          };
        }
      );

      devShells = forAllSystems (system:
        let
          tasksConfig = mkTasksConfig system;
          nix-tasks = self.packages.${system}.nix-tasks;
        in
        # Extend all dev shells to include nix-tasks binary
        builtins.mapAttrs (name: shell:
          shell.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks ];
          })
        ) tasksConfig.devShells
      );

      lib = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        import ./lib { inherit pkgs; lib = pkgs.lib; }
      );

      # Expose nix-tasks configuration for the CLI
      nixTasksConfig = forAllSystems (system: (mkTasksConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkTasksConfig system).nixTasksShells);
    };
}
