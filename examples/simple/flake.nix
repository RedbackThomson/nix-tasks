{
  description = "Simple nix-tasks example";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
        in lib.evalConfig {
          packages = {
            go = pkgs.go;
          };

          tasks = {
            build = lib.mkTask {
              description = "Build the application";
              deps = [ "go" ];
              commands = [
                "echo 'Building...'"
                "mkdir -p bin"
                "echo '#!/bin/sh' > bin/app"
                "echo 'echo Hello' >> bin/app"
                "chmod +x bin/app"
                "echo 'Done: bin/app'"
              ];
            };

            test = lib.mkTask {
              description = "Run tests";
              deps = [ "go" ];
              noCache = true;  # Always run tests, never cache results
              commands = [
                "echo 'Running tests...'"
                "echo 'All tests passed'"
              ];
            };

            clean = lib.mkTask {
              description = "Clean build artifacts";
              commands = [
                "rm -rf bin/"
                "echo 'Cleaned'"
              ];
            };
          };

          devShells = {
            default = {
              packages = [ "go" ];
              shellHook = ''
                echo "Development shell ready"
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
          # Extend the default shell to include nix-tasks
          default = config.devShells.default.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          });
        }
      );
    };
}
