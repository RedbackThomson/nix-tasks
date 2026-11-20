{
  description = "Nix-based task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      tasksLib = import ./lib { inherit (nixpkgs) lib; };
      forAllSystems = nixpkgs.lib.genAttrs tasksLib.defaultSystems;

      tasks = tasksLib.mkFlakeOutputs {
        inherit nixpkgs;
        modules = [ ./nix-tasks.nix ];
      };
    in
    tasks // {
      lib = {
        inherit (tasksLib) mkLib evalConfig mkFlakeOutputs;
      } // forAllSystems (system: tasksLib.mkLib nixpkgs.legacyPackages.${system});

      packages = forAllSystems (system: rec {
        nix-tasks = tasks.nixTasksBuilds.${system}.build;
        default = nix-tasks;
      });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.nix-tasks}/bin/nix-tasks";
        };
      });

      checks = forAllSystems (system: {
        lib = import ./lib/tests.nix {
          inherit tasksLib;
          pkgs = nixpkgs.legacyPackages.${system};
        };
      });
    };
}
