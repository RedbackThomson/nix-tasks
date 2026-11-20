{
  description = "Dotted task and shell names, and exit codes";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ tasksLib, ... }: {
          packages = {};

          tasks = {
            "lint.go" = tasksLib.mkTask {
              description = "Task whose name contains a dot";
              commands = [ "echo 'linted go'" ];
            };

            exit-four = tasksLib.mkTask {
              description = "Fails with exit code 4";
              commands = [ "exit 4" ];
            };

            exit-five = tasksLib.mkTask {
              description = "Fails with exit code 5";
              commands = [ "exit 5" ];
            };

            after-exit-four = tasksLib.mkTask {
              description = "Depends on a task that exits 4";
              depends = [ "task:exit-four" ];
              commands = [ "echo unreachable" ];
            };

            both-fail = tasksLib.mkTask {
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
        })
      ];
    };
}
