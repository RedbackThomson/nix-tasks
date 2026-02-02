{ pkgs, lib }:
let
  types = import ./types.nix { inherit lib; };
  builders = import ./builders.nix { inherit lib pkgs; };

  # Evaluate user config and generate task shells
  evalConfig = userConfig:
    let
      validated = types.validateConfig userConfig;
      shellResults = import ./shells.nix {
        inherit lib pkgs;
        config = validated;
      };
    in {
      # Config for JSON export to Go
      nixTasksConfig = {
        packages = lib.mapAttrs (name: pkg: pkg.outPath or pkg) validated.packages;
        tasks = validated.tasks;
        devShells = validated.devShells;
      };

      # Generated shells for task execution
      nixTasksShells = shellResults.taskShells;

      # User-facing dev shells
      devShells = shellResults.devShells;
    };
in {
  inherit evalConfig types builders;

  # Convenience: mkTask and other builders
  inherit (builders) mkTask mkGoTask mkDockerTask mkCompoundTask;
}
