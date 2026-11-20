{
  description = "Parallel test fixture - tasks that can run in parallel";

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
            # Four independent tasks that can all run in parallel
            task-1 = tasksLib.mkTask {
              description = "Independent task 1";
              commands = [
                "echo \"task-1 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-1 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-2 = tasksLib.mkTask {
              description = "Independent task 2";
              commands = [
                "echo \"task-2 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-2 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-3 = tasksLib.mkTask {
              description = "Independent task 3";
              commands = [
                "echo \"task-3 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-3 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-4 = tasksLib.mkTask {
              description = "Independent task 4";
              commands = [
                "echo \"task-4 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-4 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            # Final task that depends on all four
            final = tasksLib.mkTask {
              description = "Final task";
              depends = [ "task:task-1" "task:task-2" "task:task-3" "task:task-4" ];
              commands = [ "echo 'Final task - all parallel tasks completed'" ];
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
