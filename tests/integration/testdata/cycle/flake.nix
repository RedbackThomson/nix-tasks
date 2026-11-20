{
  description = "Cycle test fixture - circular dependency that should error";

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
            # Circular dependency: a -> b -> c -> a
            task-a = tasksLib.mkTask {
              description = "Task A";
              depends = [ "task:task-c" ];
              commands = [ "echo 'A'" ];
            };

            task-b = tasksLib.mkTask {
              description = "Task B";
              depends = [ "task:task-a" ];
              commands = [ "echo 'B'" ];
            };

            task-c = tasksLib.mkTask {
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
        })
      ];
    };
}
