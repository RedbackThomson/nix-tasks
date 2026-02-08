{ lib, pkgs }:
rec {
  # Shell task builder (executes shell commands in nix develop)
  mkShellTask = {
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
    noCache ? false,
    ...
  }: {
    type = "shell";
    inherit description deps depends commands script env workingDir inputs outputs continueOnError noCache;
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
    src ? ./.,
    vendorHash ? null,
    vendorSha256 ? null,
    subPackages ? null,
    ldflags ? "",
    tags ? [],
    CGO_ENABLED ? "0",
    deps ? [],
    depends ? [],
    noCache ? false,
    ...
  }@args:
    let
      # Attributes to remove for buildGoModule
      taskOnlyAttrs = [
        "name" "description" "deps" "depends" "noCache"
      ];

      # Build the Go package using buildGoModule
      goPackage = pkgs.buildGoModule ({
        inherit pname version src;
        vendorHash = if vendorHash != null then vendorHash else vendorSha256;
      } // (lib.optionalAttrs (subPackages != null) { inherit subPackages; })
        // (lib.optionalAttrs (ldflags != "") { inherit ldflags; })
        // (lib.optionalAttrs (tags != []) { inherit tags; })
        // {
          env = {
            inherit CGO_ENABLED;
          };
        }
        // (builtins.removeAttrs args (taskOnlyAttrs ++ [
          "pname" "version" "src" "vendorHash" "vendorSha256"
          "subPackages" "ldflags" "tags" "CGO_ENABLED"
        ])));
    in {
      type = "build";
      inherit description deps depends noCache;
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
    mkShellTask ({
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
    mkShellTask ({
      inherit description;
      depends = map (t: "task:${t}") tasks;
      commands = [];
    } // (builtins.removeAttrs args ["tasks" "name"]));
}
