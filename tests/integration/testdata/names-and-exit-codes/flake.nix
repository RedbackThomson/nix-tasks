{
  description = "Dotted task and shell names, and exit codes";

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
          lib = nix-tasks.lib.${system};
        in lib.evalConfig {
          packages = {};

          tasks = {
            "lint.go" = lib.mkTask {
              description = "Task whose name contains a dot";
              commands = [ "echo 'linted go'" ];
            };

            exit-four = lib.mkTask {
              description = "Fails with exit code 4";
              commands = [ "exit 4" ];
            };

            exit-five = lib.mkTask {
              description = "Fails with exit code 5";
              commands = [ "exit 5" ];
            };

            after-exit-four = lib.mkTask {
              description = "Depends on a task that exits 4";
              depends = [ "task:exit-four" ];
              commands = [ "echo unreachable" ];
            };

            both-fail = lib.mkTask {
              description = "Depends on two failing tasks";
              depends = [ "task:exit-four" "task:exit-five" ];
              commands = [ "echo unreachable" ];
            };
          };

          devShells = {
            default = {
              packages = [];
            };
            "ci.full" = {
              packages = [];
              env = { SHELL_NAME = "ci.full"; };
            };
          };
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
