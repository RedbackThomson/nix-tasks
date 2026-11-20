{
  description = "Dependencies test fixture - tasks with dependency chains";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
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
              commands = [ "echo 'Step A'" "echo step-a >> order.log" ];
            };

            step-b = lib.mkTask {
              description = "Second step";
              depends = [ "task:step-a" ];
              commands = [ "echo 'Step B'" "echo step-b >> order.log" ];
            };

            step-c = lib.mkTask {
              description = "Third step";
              depends = [ "task:step-b" ];
              commands = [ "echo 'Step C'" "echo step-c >> order.log" ];
            };

            # Diamond pattern: base -> left,right -> top
            base = lib.mkTask {
              description = "Base task";
              commands = [ "echo 'Base'" "echo base >> order.log" ];
            };

            left = lib.mkTask {
              description = "Left branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Left'" "echo left >> order.log" ];
            };

            right = lib.mkTask {
              description = "Right branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Right'" "echo right >> order.log" ];
            };

            top = lib.mkTask {
              description = "Top task (depends on left and right)";
              depends = [ "task:left" "task:right" ];
              commands = [ "echo 'Top'" "echo top >> order.log" ];
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
