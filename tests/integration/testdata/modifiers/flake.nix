{
  description = "Modifiers test fixture - task modifier helpers";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../../../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};

          # Simulate "standard" tasks that a company config would provide
          basePublish = lib.mkCompoundTask {
            description = "Publish all artifacts";
            tasks = [ "ko-publish" "helm-publish" ];
          };

          baseBuild = lib.mkTask {
            description = "Build the app";
            commands = [ "echo 'Building...'" ];
            env = { APP_NAME = "myapp"; };
          };
        in lib.evalConfig {
          packages = {};

          tasks = {
            # Leaf tasks that compound tasks depend on
            ko-publish = lib.mkTask {
              description = "Publish with ko";
              commands = [ "echo 'ko publish'" ];
            };

            helm-publish = lib.mkTask {
              description = "Publish helm chart";
              commands = [ "echo 'helm publish'" ];
            };

            helm-set-app-version = lib.mkTask {
              description = "Set app version in Helm chart";
              commands = [ "echo 'Setting app version'" ];
            };

            cleanup = lib.mkTask {
              description = "Cleanup step";
              commands = [ "echo 'Cleaning up'" ];
            };

            # Modified compound task: prepend a task dep
            publish = lib.prependTaskDeps [ "helm-set-app-version" ] basePublish;

            # Modified compound task: append a task dep
            publish-with-cleanup = lib.appendTaskDeps [ "cleanup" ] basePublish;

            # Modified shell task: pipe multiple modifications
            build = lib.pipe baseBuild [
              (lib.prependCommands [ "echo 'Pre-build step'" ])
              (lib.appendCommands [ "echo 'Post-build step'" ])
              (lib.mergeEnv { BUILD_VERSION = "1.0.0"; })
              (lib.setDescription "Build the app (customized)")
            ];

            # Modified shell task: override commands
            build-override = lib.overrideCommands
              [ "echo 'Completely new build'" ]
              baseBuild;
          };

          devShells = {
            default = {
              packages = [];
            };
          };
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
