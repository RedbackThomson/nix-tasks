{
  description = "Make compatibility layer";

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
          make = lib.compat.make;
        in lib.evalConfig {
          # No gnumake entry: make must come from stdenv.
          packages = {};

          tasks = {
            prep = lib.mkTask {
              description = "Runs before the imported sub targets";
              commands = [ "echo 'prep ran'" ];
            };

            greet = make.mkMakeTask { target = "greet"; };

            spaced-flag = make.mkMakeTask {
              target = "show-greeting";
              makeFlags = [ "GREETING=hello world" ];
            };

            spaced-makefile = make.mkMakeTask {
              target = "custom";
              makefile = "custom rules.mk";
            };
          } // make.importMakeTargets {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
