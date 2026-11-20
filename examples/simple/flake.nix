{
  description = "Simple nix-tasks example";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ pkgs, tasksLib, ... }: {
          packages = {
            go = pkgs.go;
            nix-tasks = nix-tasks.packages.${pkgs.stdenv.hostPlatform.system}.default;
          };

          tasks = {
            build = tasksLib.mkTask {
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

            test = tasksLib.mkTask {
              description = "Run tests";
              deps = [ "go" ];
              commands = [
                "echo 'Running tests...'"
                "echo 'All tests passed'"
              ];
            };

            clean = tasksLib.mkTask {
              description = "Clean build artifacts";
              commands = [
                "rm -rf bin/"
                "echo 'Cleaned'"
              ];
            };
          };

          devShells = {
            default = {
              packages = [ "go" "nix-tasks" ];
              shellHook = ''
                echo "Development shell ready"
              '';
            };
          };
        })
      ];
    } // {
      inherit (nix-tasks) packages apps;
    };
}
