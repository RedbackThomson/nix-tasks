{ lib, config, ... }:
let
  inherit (lib) mkOption types;
  ntTypes = import ./option-types.nix { inherit lib; };

  refName = lib.removePrefix "task:";

  strList = mkOption {
    type = types.listOf types.str;
    default = [ ];
    apply = lib.unique;
  };

  # Kept only so a configuration still using them gets an error (or, for
  # noCache, a warning) that names the replacement instead of "option does
  # not exist".
  removedOption = mkOption {
    type = types.raw;
    default = null;
    visible = false;
    internal = true;
  };

  cacheModule = {
    options = {
      inputs = strList // {
        description = "Globs, relative to the flake root, of files whose content determines the outputs. `!pattern` excludes.";
      };
      # Required, so that "this task writes nothing" is a decision rather than
      # an omission; a cache block missing it is how false hits happen.
      outputs = mkOption {
        type = types.nullOr (types.listOf types.str);
        default = null;
        apply = v: if v == null then null else lib.unique v;
        description = "Globs of the files the task writes, verified on every hit. `[ ]` for tasks that only check.";
      };
      env = strList // { description = "Environment variables whose values affect the outputs."; };
      maxEntries = mkOption {
        type = types.nullOr types.ints.positive;
        default = null;
        description = "Cache entries kept for this task. Overrides `cacheSettings.maxEntriesPerTask`.";
      };
    };
  };

  taskModule = { name, ... }: {
    options = {
      description = mkOption {
        type = types.str;
        default = "";
      };
      label = mkOption {
        type = types.nullOr types.nonEmptyStr;
        default = null;
        description = "Short name shown in streamed output in place of a long task name.";
      };
      type = mkOption {
        type = types.enum [ "shell" "build" ];
        default = "shell";
        description = "shell tasks run commands in a nix develop shell; build tasks build `derivation`.";
      };
      deps = strList // { description = "Names of entries in `packages` available to the task."; };
      depends = strList // { description = "Tasks (`task:<name>`) that must finish first."; };
      after = strList // { description = "Tasks (`task:<name>`) after which this task runs automatically."; };
      commands = mkOption {
        type = ntTypes.commands;
        default = [ ];
      };
      script = mkOption {
        type = types.nullOr ntTypes.script;
        default = null;
      };
      env = mkOption {
        type = types.attrsOf types.str;
        default = { };
      };
      workingDir = mkOption {
        type = types.nullOr types.str;
        default = null;
      };
      continueOnError = mkOption {
        type = types.bool;
        default = false;
      };
      cache = mkOption {
        type = types.nullOr (types.submodule cacheModule);
        default = null;
        description = "Enables the task cache: the task is skipped while its inputs, definition, toolchain and outputs are unchanged.";
      };
      inputs = removedOption;
      outputs = removedOption;
      noCache = removedOption;
      derivation = mkOption {
        type = types.nullOr types.package;
        default = null;
        description = "Derivation built by a build task.";
      };
    };
  };

  shellModule = {
    options = {
      extends = mkOption {
        type = types.nullOr types.str;
        default = null;
      };
      packages = strList;
      env = mkOption {
        type = types.attrsOf types.str;
        default = { };
      };
      shellHook = mkOption {
        type = types.lines;
        default = "";
      };
    };
  };

  shellChain = import ./shell-chain.nix { inherit lib; } config.devShells;

  taskAssertions = name: task:
    let
      unknownTasks = field: builtins.filter (ref: !(config.tasks ? ${refName ref})) task.${field};
      selfRefs = field: builtins.filter (ref: refName ref == name) task.${field};
      isBuild = task.type == "build";
      buildOnly = field: isBuild && task.${field} != [ ] && task.${field} != null;
    in
    [
      {
        assertion = unknownTasks "depends" == [ ];
        message = "task '${name}': depends on unknown task(s) ${toString (unknownTasks "depends")}";
      }
      {
        assertion = unknownTasks "after" == [ ];
        message = "task '${name}': runs after unknown task(s) ${toString (unknownTasks "after")}";
      }
      {
        assertion = selfRefs "depends" == [ ] && selfRefs "after" == [ ];
        message = "task '${name}': depends on or runs after itself";
      }
      {
        assertion = builtins.all (dep: config.packages ? ${dep}) task.deps;
        message = "task '${name}': unknown package(s) in deps: ${toString (builtins.filter (dep: !(config.packages ? ${dep})) task.deps)}";
      }
      {
        assertion = task.commands == [ ] || task.script == null;
        message = "task '${name}': sets both `commands` and `script`; set only one";
      }
      {
        assertion = !(buildOnly "commands" || buildOnly "script" || (isBuild && task.workingDir != null));
        message = "task '${name}': build tasks run `nix build` on their derivation, so `commands`, `script` and `workingDir` have no effect; remove them";
      }
      {
        assertion = task.inputs == null && task.outputs == null;
        message = "task '${name}': `inputs` and `outputs` moved to `cache.inputs` and `cache.outputs`, and `cache.outputs` is now required";
      }
    ] ++ lib.optionals (task.cache != null) [
      {
        assertion = !isBuild;
        message = "task '${name}': build tasks cannot set `cache`; Nix already decides whether a derivation needs rebuilding";
      }
      {
        assertion = task.commands != [ ] || task.script != null;
        message = "task '${name}': sets `cache` but has no `commands` or `script` to cache";
      }
      {
        assertion = task.cache.inputs != [ ];
        message = "task '${name}': `cache.inputs` must list at least one pattern";
      }
      {
        assertion = task.cache.outputs != null;
        message = "task '${name}': `cache.outputs` is required; use `[ ]` for a task that writes no files";
      }
    ];

  shellAssertions = name: shell:
    let chain = shellChain name;
    in [
      {
        assertion = shell.extends == null || config.devShells ? ${shell.extends};
        message = "devShell '${name}': extends unknown shell '${toString shell.extends}'";
      }
      {
        assertion = chain.cycle == null;
        message = "devShell '${name}': circular inheritance ${lib.concatStringsSep " -> " (if chain.cycle == null then [ ] else chain.cycle)}";
      }
      {
        assertion = builtins.all (pkg: config.packages ? ${pkg}) shell.packages;
        message = "devShell '${name}': unknown package(s) ${toString (builtins.filter (pkg: !(config.packages ? ${pkg})) shell.packages)}";
      }
    ];
in
{
  options = {
    packages = mkOption {
      type = types.lazyAttrsOf types.package;
      default = { };
      description = "Packages that tasks and dev shells refer to by name.";
    };
    tasks = mkOption {
      type = types.attrsOf (types.submodule taskModule);
      default = { };
    };
    devShells = mkOption {
      type = types.attrsOf (types.submodule shellModule);
      default = { };
    };
    cacheSettings = mkOption {
      type = types.submodule {
        options.maxEntriesPerTask = mkOption {
          type = types.nullOr types.ints.positive;
          default = null;
          description = "Cache entries kept per task (default 8). `cache.maxEntries` and NIX_TASKS_CACHE_MAX_ENTRIES override it.";
        };
      };
      default = { };
    };
    assertions = mkOption {
      type = types.listOf types.unspecified;
      internal = true;
      default = [ ];
    };
  };

  config.assertions =
    lib.concatLists (lib.mapAttrsToList taskAssertions config.tasks)
    ++ lib.concatLists (lib.mapAttrsToList shellAssertions config.devShells);
}
