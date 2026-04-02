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

      # Serialize tasks for JSON export
      serializedTasks = lib.mapAttrs (name: task:
        if task.type or "shell" == "build"
        then
          let
            # Get output names from the derivation's outputs attribute
            # This is a list like ["out"] or ["out" "bin" "dev"]
            outputNames = task.derivation.outputs or ["out"];

            # Convert to attrset: output name -> store path
            outputs = lib.listToAttrs (map (outputName: {
              name = outputName;
              value = "${task.derivation.${outputName}}";
            }) outputNames);
          in task // {
            # Store derivation path for nix build
            drvPath = task.derivation.drvPath;

            # Serialize all outputs
            inherit outputs;

            # Preserve metadata
            derivationName = task.derivation.name or name;

            # Remove derivation field (not JSON-serializable)
            derivation = null;
          }
        else task
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
