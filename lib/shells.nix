{ lib, pkgs, config }:
let
  # Detect circular inheritance
  checkCircular = name: visited:
    if builtins.elem name visited
    then throw "Circular shell inheritance: ${builtins.concatStringsSep " -> " (visited ++ [name])}"
    else visited ++ [name];

  # Resolve shell inheritance chain
  resolveShell = name: shell: visited:
    let
      newVisited = checkCircular name visited;
      parent = if shell.extends or null != null
        then resolveShell shell.extends config.devShells.${shell.extends} newVisited
        else { packages = []; env = {}; shellHook = ""; };

      resolvedPackages = parent.packages ++
        (map (p: config.packages.${p}) (shell.packages or []));
      resolvedEnv = parent.env // (shell.env or {});
      resolvedHook = lib.strings.concatStringsSep "\n" (
        lib.filter (s: s != "") [parent.shellHook (shell.shellHook or "")]
      );
    in {
      packages = resolvedPackages;
      env = resolvedEnv;
      shellHook = resolvedHook;
    };

  # Create mkShell from resolved shell
  mkDevShell = name: shell:
    let
      resolved = resolveShell name shell [];
    in pkgs.mkShell ({
      packages = resolved.packages;
      shellHook = resolved.shellHook;
    } // lib.mapAttrs' (k: v: lib.nameValuePair k v) resolved.env);

  # Create minimal shell for a task (just its deps)
  mkTaskShell = name: task:
    pkgs.mkShell {
      packages = map (dep: config.packages.${dep}) (task.deps or []);
    };

in {
  devShells = lib.mapAttrs mkDevShell (config.devShells or {});
  taskShells = lib.mapAttrs mkTaskShell (config.tasks or {});
}
