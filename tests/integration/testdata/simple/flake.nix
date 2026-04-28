{
  description = "Simple test fixture - basic tasks without dependencies";

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
            hello = lib.mkTask {
              description = "Say hello";
              commands = [ "echo 'Hello, World!'" ];
              # Declared so this task is cacheable; cache tests rely on it.
              inputs = [ "flake.nix" ];
            };

            goodbye = lib.mkTask {
              description = "Say goodbye";
              commands = [ "echo 'Goodbye!'" ];
            };

            multi-line = lib.mkTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
