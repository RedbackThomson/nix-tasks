{
  description = "Cycle test fixture - circular dependency that should error";

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
            # Circular dependency: a -> b -> c -> a
            task-a = lib.mkTask {
              description = "Task A";
              depends = [ "task:task-c" ];
              commands = [ "echo 'A'" ];
            };

            task-b = lib.mkTask {
              description = "Task B";
              depends = [ "task:task-a" ];
              commands = [ "echo 'B'" ];
            };

            task-c = lib.mkTask {
              description = "Task C";
              depends = [ "task:task-b" ];
              commands = [ "echo 'C'" ];
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
