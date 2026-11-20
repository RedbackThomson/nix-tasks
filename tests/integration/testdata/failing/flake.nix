{
  description = "Failing test fixture - tasks that fail for error handling tests";

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
            # A task that always fails
            fail = tasksLib.mkTask {
              description = "A task that fails";
              commands = [
                "echo 'About to fail...'"
                "exit 1"
              ];
            };

            # A task that succeeds
            succeed = tasksLib.mkTask {
              description = "A task that succeeds";
              commands = [ "echo 'Success!'" ];
            };

            # A task that fails but has continueOnError
            fail-continue = tasksLib.mkTask {
              description = "A task that fails but continues";
              continueOnError = true;
              commands = [
                "echo 'This will fail but continue...'"
                "exit 1"
              ];
            };

            # Task that depends on the failing task
            after-fail = tasksLib.mkTask {
              description = "Depends on fail";
              depends = [ "task:fail" ];
              commands = [ "echo 'After fail'" "echo after-fail >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            # Task that depends on the continue task
            after-continue = tasksLib.mkTask {
              description = "Depends on fail-continue";
              depends = [ "task:fail-continue" ];
              commands = [ "echo 'After continue'" "echo after-continue >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            # Independent tasks for parallel failure testing
            independent-a = tasksLib.mkTask {
              description = "Independent A";
              commands = [ "echo 'Independent A'" "echo independent-a >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            independent-b = tasksLib.mkTask {
              description = "Independent B (fails)";
              commands = [ "exit 1" ];
            };

            independent-c = tasksLib.mkTask {
              description = "Independent C";
              commands = [ "echo 'Independent C'" "echo independent-c >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            parallel-test = tasksLib.mkTask {
              description = "Depends on independent tasks";
              depends = [ "task:independent-a" "task:independent-b" "task:independent-c" ];
              commands = [ "echo 'All done'" "echo parallel-test >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
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
