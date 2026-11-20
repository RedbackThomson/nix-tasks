{ lib, pkgs }:
rec {
  # Shell task builder (executes shell commands in nix develop)
  mkShellTask = {
    name ? null,
    description ? "",
    deps ? [],
    depends ? [],
    after ? [],
    commands ? [],
    script ? null,
    env ? {},
    workingDir ? null,
    inputs ? [],
    outputs ? {},
    continueOnError ? false,
    noCache ? false,
    ...
  }: {
    type = "shell";
    inherit description deps depends after commands script env workingDir inputs outputs continueOnError noCache;
  };

  # Backward compatibility: mkTask becomes mkShellTask
  mkTask = mkShellTask;

  # Go-specific task builder using buildGoModule
  # Creates a build task that uses Nix's buildGoModule
  mkGoTask = {
    name ? null,
    description ? "Build ${pname}",
    pname ? name,
    version ? "0.1.0",
    src,
    vendorHash ? null,
    vendorSha256 ? null,
    subPackages ? null,
    ldflags ? "",
    tags ? [],
    CGO_ENABLED ? "0",
    env ? {},
    deps ? [],
    depends ? [],
    after ? [],
    workingDir ? null,
    continueOnError ? false,
    noCache ? false,
    ...
  }@args:
    let
      # Attributes to remove for buildGoModule
      taskOnlyAttrs = [
        "name" "description" "deps" "depends" "after" "workingDir" "continueOnError" "noCache"
      ];

      goEnv = { inherit CGO_ENABLED; } // env;

      # Build the Go package using buildGoModule
      goPackage = pkgs.buildGoModule ({
        inherit pname version src;
        vendorHash = if vendorHash != null then vendorHash else vendorSha256;
      } // (lib.optionalAttrs (subPackages != null) { inherit subPackages; })
        // (lib.optionalAttrs (ldflags != "") { inherit ldflags; })
        // (lib.optionalAttrs (tags != []) { inherit tags; })
        // (builtins.removeAttrs args (taskOnlyAttrs ++ [
          "pname" "version" "src" "vendorHash" "vendorSha256"
          "subPackages" "ldflags" "tags" "CGO_ENABLED" "env"
        ]))
        // { env = goEnv; });
    in {
      type = "build";
      inherit description deps depends after workingDir continueOnError noCache;
      env = goEnv;
      derivation = goPackage;
    };

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
    mkShellTask ((builtins.removeAttrs args ["image" "tag" "context" "dockerfile" "name" "deps"]) // {
      inherit description;
      deps = lib.unique (["docker"] ++ deps);
      commands = args.commands or [
        "docker build -t ${image}:${tag} -f ${dockerfile} ${context}"
      ];
    });

  # Compound task (groups other tasks)
  mkCompoundTask = {
    name ? null,
    description ? "",
    tasks,
    depends ? [],
    ...
  }@args:
    mkShellTask ((builtins.removeAttrs args ["tasks" "name" "depends"]) // {
      inherit description;
      depends = (map (t: "task:${t}") tasks) ++ depends;
      commands = args.commands or [];
    });
}
