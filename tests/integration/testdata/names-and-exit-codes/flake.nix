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
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
