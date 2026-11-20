{
  description = "After-hooks test fixture - tasks with after-hook relationships";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ tasksLib, ... }: {
          packages = {};

          tasks = {
            # Basic after-hook: hook runs after build
            build = tasksLib.mkTask {
              description = "Build the project";
              commands = [ "echo 'Building...'" ];
            };

            post-build = tasksLib.mkTask {
              description = "Post-build hook";
              after = [ "task:build" ];
              commands = [ "echo 'Post-build hook ran'" ];
            };

            # After-hook not triggered: unrelated task should not pull in hooks
            unrelated = tasksLib.mkTask {
              description = "Unrelated task";
              commands = [ "echo 'Unrelated'" ];
            };

            # Transitive after-hooks: c hooks onto b, b hooks onto a
            chain-a = tasksLib.mkTask {
              description = "Chain start";
              commands = [ "echo 'Chain A'" ];
            };

            chain-b = tasksLib.mkTask {
              description = "Chain middle (hooks onto chain-a)";
              after = [ "task:chain-a" ];
              commands = [ "echo 'Chain B'" ];
            };

            chain-c = tasksLib.mkTask {
              description = "Chain end (hooks onto chain-b)";
              after = [ "task:chain-b" ];
              commands = [ "echo 'Chain C'" ];
            };

            # After-hook with its own depends
            setup = tasksLib.mkTask {
              description = "Setup task";
              commands = [ "echo 'Setup'" ];
            };

            deploy = tasksLib.mkTask {
              description = "Deploy task";
              commands = [ "echo 'Deploying...'" ];
            };

            post-deploy = tasksLib.mkTask {
              description = "Post-deploy hook with its own dependency";
              after = [ "task:deploy" ];
              depends = [ "task:setup" ];
              commands = [ "echo 'Post-deploy (needed setup first)'" ];
            };

            # Multiple hooks on the same target
            test = tasksLib.mkTask {
              description = "Run tests";
              commands = [ "echo 'Testing...'" ];
            };

            coverage-report = tasksLib.mkTask {
              description = "Generate coverage report after tests";
              after = [ "task:test" ];
              commands = [ "echo 'Coverage report'" ];
            };

            notify = tasksLib.mkTask {
              description = "Send notification after tests";
              after = [ "task:test" ];
              commands = [ "echo 'Notifying...'" ];
            };
          };

          devShells = {
            default = {
              packages = [];
            };
          };
        })
      ];
    };
}
