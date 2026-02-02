{ lib, pkgs }:
rec {
  # Generic task builder
  mkTask = {
    name ? null,
    description ? "",
    deps ? [],
    depends ? [],
    commands ? [],
    script ? null,
    env ? {},
    workingDir ? null,
    inputs ? [],
    outputs ? [],
    continueOnError ? false,
    ...
  }: {
    inherit description deps depends commands script env workingDir inputs outputs continueOnError;
  };

  # Go-specific task builder
  mkGoTask = {
    name ? null,
    description ? "",
    command ? "build",
    output ? null,
    packages ? [],
    ldflags ? "",
    deps ? [],
    ...
  }@args:
    mkTask ({
      inherit description;
      deps = ["go"] ++ packages ++ deps;
      commands = [
        ("go ${command}" +
          lib.optionalString (ldflags != "") " -ldflags '${ldflags}'" +
          lib.optionalString (output != null) " -o ${output}" +
          " ./...")
      ];
    } // (builtins.removeAttrs args ["command" "output" "packages" "ldflags" "name"]));

  # Docker task builder
  mkDockerTask = {
    name ? null,
    description ? "",
    image,
    tag ? "latest",
    context ? ".",
    dockerfile ? "Dockerfile",
    deps ? [],
    ...
  }@args:
    mkTask ({
      inherit description;
      deps = ["docker"] ++ deps;
      commands = [
        "docker build -t ${image}:${tag} -f ${dockerfile} ${context}"
      ];
    } // (builtins.removeAttrs args ["image" "tag" "context" "dockerfile" "name"]));

  # Compound task (groups other tasks)
  mkCompoundTask = {
    name ? null,
    description ? "",
    tasks,
    ...
  }@args:
    mkTask ({
      inherit description;
      depends = map (t: "task:${t}") tasks;
      commands = [];
    } // (builtins.removeAttrs args ["tasks" "name"]));
}
