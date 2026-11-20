{
  description = "Environment test fixture - tasks with environment variables";

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
            print-env = tasksLib.mkTask {
              description = "Print environment variables";
              env = {
                MY_VAR = "hello";
                ANOTHER_VAR = "world";
              };
              commands = [
                "echo \"MY_VAR=$MY_VAR\""
                "echo \"ANOTHER_VAR=$ANOTHER_VAR\""
              ];
            };

            check-env = tasksLib.mkTask {
              description = "Check environment variables are set";
              env = {
                EXPECTED_VALUE = "test123";
              };
              commands = [
                "test \"$EXPECTED_VALUE\" = \"test123\" && echo 'Environment check passed'"
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
