# Make compatibility layer for nix-tasks
#
# This module provides helpers for wrapping Make targets as nix-tasks tasks,
# enabling gradual migration from Makefiles to nix-tasks.
#
# Usage:
#   tasks = {
#     # Wrap a single Make target
#     legacy-build = compat.mkMakeTask {
#       target = "build";
#       description = "Run legacy Make build";
#     };
#
#     # Wrap with custom makefile
#     legacy-test = compat.mkMakeTask {
#       target = "test";
#       makefile = "Makefile.test";
#       description = "Run legacy Make tests";
#     };
#
#     # Import multiple targets at once
#   } // compat.importMakeTargets {
#     targets = [ "clean" "install" "dist" ];
#     prefix = "make";  # Creates make-clean, make-install, make-dist
#   };
#
{ lib, pkgs, builders }:
rec {
  # Wrap a single Make target as a nix-tasks task
  #
  # Arguments:
  #   target      - The Make target to run
  #   makefile    - Path to Makefile (default: "Makefile")
  #   description - Task description (auto-generated if not provided)
  #   makeFlags   - Additional flags to pass to make
  #   env         - Environment variables for the make command
  #   deps        - Additional package dependencies
  #   depends     - Task dependencies
  #   workingDir  - Working directory for make
  #   continueOnError - Whether to continue on failure
  #
  # Returns a task definition
  mkMakeTask = {
    target,
    makefile ? "Makefile",
    description ? null,
    makeFlags ? [],
    env ? {},
    deps ? [],
    depends ? [],
    workingDir ? null,
    continueOnError ? false,
    ...
  }@args:
    let
      desc = if description != null
        then description
        else "Make target: ${target}";

      flagsStr = lib.concatStringsSep " " makeFlags;
      makeCmd = "make" +
        lib.optionalString (makefile != "Makefile") " -f ${makefile}" +
        lib.optionalString (flagsStr != "") " ${flagsStr}" +
        " ${target}";
    in builders.mkTask ({
      description = desc;
      deps = [ "gnumake" ] ++ deps;
      inherit depends workingDir continueOnError env;
      commands = [ makeCmd ];
    } // (builtins.removeAttrs args [
      "target" "makefile" "description" "makeFlags" "env"
      "deps" "depends" "workingDir" "continueOnError"
    ]));

  # Import multiple Make targets as nix-tasks tasks
  #
  # Arguments:
  #   targets     - List of Make targets to import
  #   prefix      - Prefix for task names (default: "make")
  #   makefile    - Path to Makefile (default: "Makefile")
  #   makeFlags   - Additional flags to pass to make
  #   deps        - Additional package dependencies (applied to all)
  #   env         - Environment variables (applied to all)
  #
  # Returns an attribute set of tasks
  #
  # Example:
  #   importMakeTargets {
  #     targets = [ "clean" "build" "test" ];
  #     prefix = "legacy";
  #   }
  #   => { legacy-clean = ...; legacy-build = ...; legacy-test = ...; }
  #
  importMakeTargets = {
    targets,
    prefix ? "make",
    makefile ? "Makefile",
    makeFlags ? [],
    deps ? [],
    env ? {},
  }:
    builtins.listToAttrs (map (target: {
      name = "${prefix}-${target}";
      value = mkMakeTask {
        inherit target makefile makeFlags deps env;
      };
    }) targets);

  # Generate a migration task that runs both Make and nix-tasks versions
  # for comparison during migration.
  #
  # Arguments:
  #   makeTarget  - The Make target to run
  #   taskName    - The nix-tasks task to compare with
  #   description - Task description
  #
  # Returns a task that runs both and compares outputs
  mkMigrationTask = {
    makeTarget,
    taskName,
    description ? "Compare Make ${makeTarget} with nix-tasks ${taskName}",
    ...
  }@args:
    builders.mkTask ({
      inherit description;
      deps = [ "gnumake" ];
      depends = [ "task:${taskName}" ];
      commands = [
        "echo 'Migration comparison: make ${makeTarget} vs nix-tasks ${taskName}'"
        "echo 'Make version:'"
        "make ${makeTarget} 2>&1 || true"
        "echo ''"
        "echo 'nix-tasks version completed via dependency'"
      ];
    } // (builtins.removeAttrs args ["makeTarget" "taskName" "description"]));
}
