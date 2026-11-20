{
  description = "Dependencies test fixture - tasks with dependency chains";

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
            # Linear chain: a -> b -> c
            step-a = tasksLib.mkTask {
              description = "First step";
              commands = [ "echo 'Step A'" "echo step-a >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            step-b = tasksLib.mkTask {
              description = "Second step";
              depends = [ "task:step-a" ];
              commands = [ "echo 'Step B'" "echo step-b >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            step-c = tasksLib.mkTask {
              description = "Third step";
              depends = [ "task:step-b" ];
              commands = [ "echo 'Step C'" "echo step-c >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            # Diamond pattern: base -> left,right -> top
            base = tasksLib.mkTask {
              description = "Base task";
              commands = [ "echo 'Base'" "echo base >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            left = tasksLib.mkTask {
              description = "Left branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Left'" "echo left >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            right = tasksLib.mkTask {
              description = "Right branch";
              depends = [ "task:base" ];
              commands = [ "echo 'Right'" "echo right >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            top = tasksLib.mkTask {
              description = "Top task (depends on left and right)";
              depends = [ "task:left" "task:right" ];
              commands = [ "echo 'Top'" "echo top >> \"$NIX_TASKS_TEST_LOG_DIR/order.log\"" ];
            };

            # Compound task
            all = tasksLib.mkCompoundTask {
              description = "Run all steps";
              tasks = [ "step-c" "top" ];
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
