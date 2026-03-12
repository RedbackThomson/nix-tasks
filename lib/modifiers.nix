# Task modifier helpers for nix-tasks
#
# These functions operate on already-built task attr sets (the output of
# mkTask, mkShellTask, mkCompoundTask, etc.) and return modified copies.
#
# All modifiers use curried form: modification args first, task last.
# This enables partial application and composition via `pipe`.
#
# Usage:
#   inherit (modifiers) prependTaskDeps appendTaskDeps prependDeps appendDeps
#                        prependCommands appendCommands mergeEnv pipe;
#
#   # Prepend a task dependency to a compound task
#   publish = prependTaskDeps ["helm-set-app-version"] standards.tasks.publish;
#
#   # Chain multiple modifications
#   publish = pipe standards.tasks.publish [
#     (prependTaskDeps ["helm-set-app-version"])
#     (appendCommands ["echo 'Done'"])
#     (mergeEnv { NOTIFY = "true"; })
#   ];
#
{ lib }:
let
  taskDependencyPrefix = "task:";

  # Convert a list of task names to dependency references
  taskRefs = names: map (t: "${taskDependencyPrefix}${t}") names;
in
rec {
  # Task dependency modifiers (depends field with "task:" prefix)

  # Prepend task dependencies
  # prependTaskDeps ["setup"] myTask => adds "task:setup" before existing depends
  prependTaskDeps = names: task:
    task // { depends = (taskRefs names) ++ (task.depends or []); };

  # Append task dependencies
  appendTaskDeps = names: task:
    task // { depends = (task.depends or []) ++ (taskRefs names); };

  # Override all task dependencies
  overrideTaskDeps = names: task:
    task // { depends = taskRefs names; };

  # Package dependency modifiers (deps field)

  prependDeps = packages: task:
    task // { deps = packages ++ (task.deps or []); };

  appendDeps = packages: task:
    task // { deps = (task.deps or []) ++ packages; };

  overrideDeps = packages: task:
    task // { deps = packages; };

  # Command modifiers (commands field)
  # These handle tasks that use `script` instead of `commands` by converting
  # the script into a commands entry, so prepended/appended commands run in
  # the expected order.

  # Resolve a task's effective commands list, folding `script` into `commands`
  # if present. Returns a plain list of command strings.
  effectiveCommands = task:
    if (task.script or null) != null && task.script != ""
    then [ task.script ]
    else (task.commands or []);

  # Clear the script field so the runner uses commands instead
  clearScript = task: task // { script = null; };

  prependCommands = cmds: task:
    clearScript (task // { commands = cmds ++ (effectiveCommands task); });

  appendCommands = cmds: task:
    clearScript (task // { commands = (effectiveCommands task) ++ cmds; });

  overrideCommands = cmds: task:
    clearScript (task // { commands = cmds; });

  # Environment variable modifiers (env field)

  # Merge environment variables (new values override existing on conflict)
  mergeEnv = vars: task:
    task // { env = (task.env or {}) // vars; };

  # Override all environment variables
  overrideEnv = vars: task:
    task // { env = vars; };

  # Input modifiers

  appendInputs = patterns: task:
    task // { inputs = (task.inputs or []) ++ patterns; };

  overrideInputs = patterns: task:
    task // { inputs = patterns; };

  # Metadata modifiers

  setDescription = desc: task:
    task // { description = desc; };

  setWorkingDir = dir: task:
    task // { workingDir = dir; };

  setNoCache = value: task:
    task // { noCache = value; };

  setContinueOnError = value: task:
    task // { continueOnError = value; };

  # Apply a list of modifier functions to a task sequentially
  #
  # pipe standards.tasks.publish [
  #   (prependTaskDeps ["helm-set-app-version"])
  #   (appendCommands ["echo 'Done'"])
  #   (mergeEnv { NOTIFY = "true"; })
  # ]
  pipe = task: modifiers:
    builtins.foldl' (t: modifier: modifier t) task modifiers;
}
