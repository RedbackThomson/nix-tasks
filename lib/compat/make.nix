# Make compatibility layer for nix-tasks
#
# This module provides helpers for wrapping Make targets as nix-tasks tasks,
# enabling gradual migration from Makefiles to nix-tasks.
#
# Usage, in a module that receives `tasksLib`:
#   tasks = {
#     legacy-build = tasksLib.compat.make.mkMakeTask {
#       target = "build";
#       description = "Run legacy Make build";
#     };
#   } // tasksLib.compat.make.importMakeTargets {
#     targets = [ "clean" "install" "dist" ];
#     prefix = "make";  # Creates make-clean, make-install, make-dist
#   };
#
{ lib }:
rec {
  # Wrap a single Make target as a nix-tasks task
  #
  # Arguments:
  #   target      - The Make target to run
  #   makefile    - Path to Makefile (default: "Makefile")
  #   description - Task description (defaults to "Make target: <target>")
  #   makeFlags   - Additional arguments to pass to make, one per element
  #
  # Any other task field (deps, depends, env, workingDir, ...) is passed
  # through as a task field. make comes from stdenv, so it needs no package entry.
  #
  # Returns a task definition
  mkMakeTask = {
    target,
    makefile ? "Makefile",
    makeFlags ? [],
    ...
  }@args:
    let
      makeArgs = [ "make" ]
        ++ lib.optionals (makefile != "Makefile") [ "-f" makefile ]
        ++ makeFlags
        ++ [ target ];
    in (builtins.removeAttrs args [ "target" "makefile" "makeFlags" ]) // {
      description = args.description or (lib.mkDefault "Make target: ${target}");
      commands = [ (lib.escapeShellArgs makeArgs) ];
    };

  # Import multiple Make targets as nix-tasks tasks
  #
  # Arguments:
  #   targets     - List of Make targets to import
  #   prefix      - Prefix for task names (default: "make")
  #   Any other argument (makefile, makeFlags, deps, env, workingDir, ...)
  #   is passed to mkMakeTask for every target.
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
    ...
  }@args:
    builtins.listToAttrs (map (target: {
      name = "${prefix}-${target}";
      value = mkMakeTask (builtins.removeAttrs args [ "targets" "prefix" ] // { inherit target; });
    }) targets);
}
