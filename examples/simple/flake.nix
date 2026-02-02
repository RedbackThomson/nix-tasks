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
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
