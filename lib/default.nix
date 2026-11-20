{ pkgs, lib }:
let
  types = import ./types.nix { inherit lib; };
  builders = import ./builders.nix { inherit lib pkgs; };
  compose = import ./compose.nix { inherit lib; };
  modifiers = import ./modifiers.nix { inherit lib; };
  compat = import ./compat { inherit lib pkgs builders; };

  # Evaluate user config and generate task shells
  evalConfig = userConfig:
    let
      validated = types.validateConfig userConfig;
      shellResults = import ./shells.nix {
        inherit lib pkgs;
        config = validated;
      };

      isBuild = task: (task.type or "shell") == "build";

      # Build tasks are serialized without their derivation so that listing
      # tasks never instantiates it; the CLI builds them via nixTasksBuilds.
      serializedTasks = lib.mapAttrs (name: task:
        builtins.addErrorContext "while evaluating nix-tasks task '${name}'"
          (if isBuild task then builtins.removeAttrs task [ "derivation" ] else task)
      ) validated.tasks;
    in {
      # Config for JSON export to Go
      nixTasksConfig = {
        packages = lib.mapAttrs (name: pkg: pkg.outPath or pkg) validated.packages;
        tasks = serializedTasks;
        devShells = validated.devShells;
      };

      # Generated shells for task execution
      nixTasksShells = shellResults.taskShells;

      # Derivations of build tasks, built by the CLI as flake attributes
      nixTasksBuilds = lib.mapAttrs (name: task: task.derivation)
        (lib.filterAttrs (name: isBuild) validated.tasks);

      # User-facing dev shells
      devShells = shellResults.devShells;

      # Raw validated tasks (for accessing derivations)
      rawTasks = validated.tasks;
    };
in {
  inherit evalConfig types builders compose modifiers compat;

  # Convenience: mkTask and other builders
  inherit (builders) mkTask mkShellTask mkGoTask mkDockerTask mkCompoundTask;

  # Convenience: composition helpers
  inherit (compose) override append prepend extend mergeAttrs;

  # Convenience: task modifiers
  inherit (modifiers) prependTaskDeps appendTaskDeps overrideTaskDeps
                       prependAfterHooks appendAfterHooks overrideAfterHooks
                       prependDeps appendDeps overrideDeps
                       prependCommands appendCommands overrideCommands
                       mergeEnv overrideEnv
                       appendInputs overrideInputs
                       setDescription setWorkingDir setNoCache setContinueOnError
                       pipe;
}
