{
  description = "Composition fixture - a repo module customising a standards module";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        # What a company standards flake would provide.
        ({ lib, tasksLib, ... }: {
          tasks = {
            ko-publish = {
              description = "Publish with ko";
              commands = [ "echo 'ko publish'" ];
            };

            helm-publish = {
              description = "Publish helm chart";
              commands = [ "echo 'helm publish'" ];
            };

            publish = tasksLib.mkCompoundTask {
              description = "Publish all artifacts";
              tasks = [ "ko-publish" "helm-publish" ];
            };

            publish-with-cleanup = tasksLib.mkCompoundTask {
              description = "Publish all artifacts";
              tasks = [ "ko-publish" "helm-publish" ];
            };

            build = {
              description = lib.mkDefault "Build the app";
              commands = [ "echo 'Building...'" ];
              env.APP_NAME = "myapp";
            };

            build-override = {
              description = "Build the app";
              commands = [ "echo 'Building...'" ];
            };

            script-modified = {
              description = "Script-based task";
              script = ''
                echo 'Script line 1'
                echo 'Script line 2'
              '';
            };
          };

          devShells.default.packages = [ ];
        })

        # The repository's customisations.
        ({ lib, ... }: {
          tasks = {
            helm-set-app-version = {
              description = "Set app version in Helm chart";
              commands = [ "echo 'Setting app version'" ];
            };

            cleanup = {
              description = "Cleanup step";
              commands = [ "echo 'Cleaning up'" ];
            };

            publish.depends = lib.mkBefore [ "task:helm-set-app-version" ];

            publish-with-cleanup.depends = [ "task:cleanup" ];

            build = {
              description = "Build the app (customized)";
              commands = lib.mkMerge [
                (lib.mkBefore [ "echo 'Pre-build step'" ])
                (lib.mkAfter [ "echo 'Post-build step'" ])
              ];
              env.BUILD_VERSION = "1.0.0";
            };

            build-override.commands = lib.mkForce [ "echo 'Completely new build'" ];

            script-modified.script = lib.mkMerge [
              (lib.mkBefore "echo 'Before script'")
              (lib.mkAfter "echo 'After script'")
            ];
          };
        })
      ];
    };
}
