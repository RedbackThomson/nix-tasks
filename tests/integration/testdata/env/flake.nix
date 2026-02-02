{
  description = "Environment test fixture - tasks with environment variables";

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
            print-env = lib.mkTask {
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

            check-env = lib.mkTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
