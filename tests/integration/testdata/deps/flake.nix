{
  description = "Dependencies test fixture - tasks with dependency chains";

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
            # Linear chain: a -> b -> c
            step-a = lib.mkTask {
              description = "First step";
              commands = [ "echo 'Step A'" ];
            };

            step-b = lib.mkTask {
              description = "Second step";
              depends = [ "task:step-a" ];
              commands = [ "echo 'Step B'" ];
            };

            step-c = lib.mkTask {
              description = "Third step";
              depends = [ "task:step-b" ];
              commands = [ "echo 'Step C'" ];
            };

            # Diamond pattern: base -> left,right -> top
            base = lib.mkTask {
              description = "Base task";
              commands = [ "echo 'Base'" ];
            };

            left = lib.mkTask {
              description = "Left branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Left'" ];
            };

            right = lib.mkTask {
              description = "Right branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Right'" ];
            };

            top = lib.mkTask {
              description = "Top task (depends on left and right)";
              depends = [ "task:left" "task:right" ];
              commands = [ "echo 'Top'" ];
            };

            # Compound task
            all = lib.mkCompoundTask {
              description = "Run all steps";
              tasks = [ "step-c" "top" ];
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
