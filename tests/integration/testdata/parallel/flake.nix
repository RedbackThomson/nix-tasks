{
  description = "Parallel test fixture - tasks that can run in parallel";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../../../..";
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
          packages = {};

          tasks = {
            # Four independent tasks that can all run in parallel
            task-1 = lib.mkTask {
              description = "Independent task 1";
              commands = [ "echo 'Task 1 starting' && sleep 0.1 && echo 'Task 1 done'" ];
            };

            task-2 = lib.mkTask {
              description = "Independent task 2";
              commands = [ "echo 'Task 2 starting' && sleep 0.1 && echo 'Task 2 done'" ];
            };

            task-3 = lib.mkTask {
              description = "Independent task 3";
              commands = [ "echo 'Task 3 starting' && sleep 0.1 && echo 'Task 3 done'" ];
            };

            task-4 = lib.mkTask {
              description = "Independent task 4";
              commands = [ "echo 'Task 4 starting' && sleep 0.1 && echo 'Task 4 done'" ];
            };

            # Final task that depends on all four
            final = lib.mkTask {
              description = "Final task";
              depends = [ "task:task-1" "task:task-2" "task:task-3" "task:task-4" ];
              commands = [ "echo 'Final task - all parallel tasks completed'" ];
            };
          };

          devShells = {
            default = {
              packages = [];
            };
          };
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
