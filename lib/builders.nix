# Builders return partial task definitions: only the fields the caller set,
# plus builder-specific ones, so a builder used in one module merges with a
# definition of the same task in another instead of resetting its fields.
# Values a builder invents are mkDefault so any plain definition wins.
{ lib, pkgs }:
let
  taskFields = [
    "description" "label" "type" "deps" "depends" "after" "commands" "script" "env"
    "workingDir" "continueOnError" "cache" "derivation"
    # Removed fields stay routed to the task so they reach its migration error.
    "inputs" "noCache"
  ];
in
rec {
  mkTask = args: args;

  mkShellTask = mkTask;

  mkGoTask = {
    pname,
    version ? "0.1.0",
    src,
    vendorHash ? null,
    subPackages ? null,
    ldflags ? [ ],
    tags ? [ ],
    CGO_ENABLED ? "0",
    env ? { },
    ...
  }@args:
    let
      goArgs = builtins.removeAttrs args (taskFields ++ [ "CGO_ENABLED" "env" "subPackages" ]);
    in
    lib.filterAttrs (n: _: n != "env" && builtins.elem n taskFields) args // {
      type = "build";
      description = args.description or (lib.mkDefault "Build ${pname}");
      derivation = pkgs.buildGoModule (goArgs // {
        inherit version vendorHash ldflags tags;
        env = { inherit CGO_ENABLED; } // env;
      } // lib.optionalAttrs (subPackages != null) { inherit subPackages; });
    };

  mkDockerTask = {
    image,
    tag ? "latest",
    context ? ".",
    dockerfile ? "Dockerfile",
    ...
  }@args:
    builtins.removeAttrs args [ "image" "tag" "context" "dockerfile" ] // {
      deps = [ "docker" ] ++ args.deps or [ ];
      commands = args.commands or (lib.mkDefault [
        (lib.escapeShellArgs [ "docker" "build" "-t" "${image}:${tag}" "-f" dockerfile context ])
      ]);
    };

  mkCompoundTask = { tasks, ... }@args:
    builtins.removeAttrs args [ "tasks" ] // {
      depends = map (t: "task:${t}") tasks ++ args.depends or [ ];
    };
}
