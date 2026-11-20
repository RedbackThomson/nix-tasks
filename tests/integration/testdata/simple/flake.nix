{
  description = "Simple test fixture - basic tasks without dependencies";

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
            hello = tasksLib.mkTask {
              description = "Say hello";
              commands = [ "echo 'Hello, World!'" ];
              # Declared so this task is cacheable; cache tests rely on it.
              inputs = [ "flake.nix" ];
            };

            goodbye = tasksLib.mkTask {
              description = "Say goodbye";
              commands = [ "echo 'Goodbye!'" ];
            };

            multi-line = tasksLib.mkTask {
              description = "Multi-line output";
              commands = [
                "echo 'Line 1'"
                "echo 'Line 2'"
                "echo 'Line 3'"
              ];
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
