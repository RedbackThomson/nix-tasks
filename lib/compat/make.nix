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
  #   makeFlags   - Additional arguments to pass to make, one per element
  #
  # Any other task field (deps, depends, env, workingDir, ...) is passed
  # through to mkTask. make comes from stdenv, so it needs no package entry.
  #
  # Returns a task definition
  mkMakeTask = {
    target,
    makefile ? "Makefile",
    description ? null,
    makeFlags ? [],
    ...
  }@args:
    let
      makeArgs = [ "make" ]
        ++ lib.optionals (makefile != "Makefile") [ "-f" makefile ]
        ++ makeFlags
        ++ [ target ];
    in builders.mkTask ((builtins.removeAttrs args [ "target" "makefile" "makeFlags" ]) // {
      description = if description != null then description else "Make target: ${target}";
      commands = [ (lib.escapeShellArgs makeArgs) ];
    });

  # Import multiple Make targets as nix-tasks tasks
  #
  # Arguments:
  #   targets     - List of Make targets to import
  #   prefix      - Prefix for task names (default: "make")
  #   makefile    - Path to Makefile (default: "Makefile")
  #   makeFlags   - Additional arguments to pass to make
  #   deps        - Additional package dependencies (applied to all)
  #   depends     - Task dependencies (applied to all)
  #   env         - Environment variables (applied to all)
  #   workingDir  - Working directory for make (applied to all)
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
    depends ? [],
    env ? {},
    workingDir ? null,
  }:
    builtins.listToAttrs (map (target: {
      name = "${prefix}-${target}";
      value = mkMakeTask {
        inherit target makefile makeFlags deps depends env workingDir;
      };
    }) targets);
}
