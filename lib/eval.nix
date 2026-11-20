# The flake outputs of a nix-tasks configuration. Attribute names and the JSON
# shape of nixTasksConfig must match internal/config and internal/nix.
{ mkLib }:
{ pkgs, modules, specialArgs ? { } }:
let
  inherit (pkgs) lib;

  eval = lib.evalModules {
    modules = [ ./module.nix ] ++ modules;
    specialArgs = { inherit pkgs; tasksLib = mkLib pkgs; } // specialArgs;
  };
  cfg = eval.config;

  failed = map (a: a.message) (builtins.filter (a: !a.assertion) cfg.assertions);
  checked = value:
    if failed == [ ] then value
    else throw "nix-tasks configuration is invalid:\n  - ${lib.concatStringsSep "\n  - " failed}";

  shellTasks = lib.filterAttrs (_: task: task.type == "shell") cfg.tasks;
  buildTasks = lib.filterAttrs (_: task: task.type == "build") cfg.tasks;

  # Only cacheable tasks carry their shell's drvPath in nixTasksConfig, since
  # computing it instantiates the shell and every `list` would pay for that.
  cacheable = task: task.type == "shell" && task.inputs != [ ] && !task.noCache;

  taskShells = lib.mapAttrs (_: task:
    pkgs.mkShell { packages = map (dep: cfg.packages.${dep}) task.deps; }
  ) shellTasks;

  taskShellDrvs = lib.mapAttrs (_: shell: shell.drvPath) taskShells;

  withoutNulls = lib.filterAttrs (_: v: v != null);

  serializeTask = name: task:
    withoutNulls (builtins.removeAttrs task [ "derivation" "_module" ])
    // lib.optionalAttrs (cacheable task) { shellDrvPath = taskShellDrvs.${name}; };

  serializeShell = _: shell: withoutNulls (builtins.removeAttrs shell [ "_module" ]);

  shellChain = import ./shell-chain.nix { inherit lib; } cfg.devShells;

  devShell = name: _:
    let
      lineage = map (n: cfg.devShells.${n}) (lib.reverseList (shellChain name).chain);
      packages = lib.unique (lib.concatMap (s: s.packages) lineage);
      hooks = builtins.filter (h: h != "") (map (s: s.shellHook) lineage);
    in
    pkgs.mkShell {
      packages = map (p: cfg.packages.${p}) packages;
      shellHook = lib.concatStringsSep "\n" hooks;
      env = lib.foldl' (acc: s: acc // s.env) { } lineage;
    };
in
{
  inherit (eval) config options;

  nixTasksConfig = checked {
    packages = builtins.attrNames cfg.packages;
    tasks = lib.mapAttrs serializeTask cfg.tasks;
    devShells = lib.mapAttrs serializeShell cfg.devShells;
  };

  nixTasksShellDrvs = checked taskShellDrvs;

  nixTasksBuilds = checked (lib.mapAttrs (name: task:
    if task.derivation == null
    then throw "nix-tasks: build task '${name}' has no `derivation`"
    else task.derivation
  ) buildTasks);

  devShells = checked (lib.mapAttrs devShell cfg.devShells);
}
