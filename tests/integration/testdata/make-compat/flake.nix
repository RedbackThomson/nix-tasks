{
  description = "Make compatibility layer";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ tasksLib, ... }: {
          # No gnumake entry: make must come from stdenv.
          packages = {};

          tasks = {
            prep = tasksLib.mkTask {
              description = "Runs before the imported sub targets";
              commands = [ "echo 'prep ran'" ];
            };

            greet = tasksLib.compat.make.mkMakeTask { target = "greet"; };

            spaced-flag = tasksLib.compat.make.mkMakeTask {
              target = "show-greeting";
              makeFlags = [ "GREETING=hello world" ];
            };

            spaced-makefile = tasksLib.compat.make.mkMakeTask {
              target = "custom";
              makefile = "custom rules.mk";
            };
          } // tasksLib.compat.make.importMakeTargets {
            targets = [ "where" ];
            prefix = "sub";
            depends = [ "task:prep" ];
            workingDir = "sub";
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
