{
  description = "Failing test fixture - tasks that fail for error handling tests";

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
            # A task that always fails
            fail = lib.mkTask {
              description = "A task that fails";
              commands = [
                "echo 'About to fail...'"
                "exit 1"
              ];
            };

            # A task that succeeds
            succeed = lib.mkTask {
              description = "A task that succeeds";
              commands = [ "echo 'Success!'" ];
            };

            # A task that fails but has continueOnError
            fail-continue = lib.mkTask {
              description = "A task that fails but continues";
              continueOnError = true;
              commands = [
                "echo 'This will fail but continue...'"
                "exit 1"
              ];
            };

            # Task that depends on the failing task
            after-fail = lib.mkTask {
              description = "Depends on fail";
              depends = [ "task:fail" ];
              commands = [ "echo 'After fail'" "echo after-fail >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            # Task that depends on the continue task
            after-continue = lib.mkTask {
              description = "Depends on fail-continue";
              depends = [ "task:fail-continue" ];
              commands = [ "echo 'After continue'" "echo after-continue >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            # Independent tasks for parallel failure testing
            independent-a = lib.mkTask {
              description = "Independent A";
              commands = [ "echo 'Independent A'" "echo independent-a >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            independent-b = lib.mkTask {
              description = "Independent B (fails)";
              commands = [ "exit 1" ];
            };

            independent-c = lib.mkTask {
              description = "Independent C";
              commands = [ "echo 'Independent C'" "echo independent-c >> \"$NIX_TASKS_TEST_LOG_DIR/ran.log\"" ];
            };

            parallel-test = lib.mkTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
