{
  description = "Shells test fixture - dev shells with inheritance";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ pkgs, tasksLib, ... }: {
          packages = {
            jq = pkgs.jq;
            curl = pkgs.curl;
          };

          tasks = {
            check-jq = tasksLib.mkTask {
              description = "Check jq is available";
              deps = [ "jq" ];
              commands = [ "jq --version" ];
            };

            check-curl = tasksLib.mkTask {
              description = "Check curl is available";
              deps = [ "curl" ];
              commands = [ "curl --version | head -1" ];
            };
          };

          devShells = {
            minimal = {
              packages = [ "jq" ];
              env = {
                SHELL_TYPE = "minimal";
              };
            };

            extended = {
              extends = "minimal";
              packages = [ "curl" ];
              env = {
                SHELL_TYPE = "extended";
              };
            };

            default = {
              extends = "extended";
              shellHook = ''
                echo "Welcome to the test shell"
              '';
            };
          };
        })
      ];
    };
}
