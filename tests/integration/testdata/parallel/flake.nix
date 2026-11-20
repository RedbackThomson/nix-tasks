{
  description = "Parallel test fixture - tasks that can run in parallel";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
        in lib.evalConfig {
          packages = {};

          tasks = {
            # Four independent tasks that can all run in parallel
            task-1 = lib.mkTask {
              description = "Independent task 1";
              commands = [
                "echo \"task-1 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-1 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-2 = lib.mkTask {
              description = "Independent task 2";
              commands = [
                "echo \"task-2 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-2 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-3 = lib.mkTask {
              description = "Independent task 3";
              commands = [
                "echo \"task-3 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-3 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            task-4 = lib.mkTask {
              description = "Independent task 4";
              commands = [
                "echo \"task-4 start $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
                "sleep 1"
                "echo \"task-4 end $(date +%s%3N)\" >> \"$NIX_TASKS_TEST_LOG_DIR/timings.log\""
              ];
            };

            # Final task that depends on all four
            final = lib.mkTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
