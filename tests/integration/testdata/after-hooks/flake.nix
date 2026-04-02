{
  description = "After-hooks test fixture - tasks with after-hook relationships";

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
        in lib.evalConfig {
          packages = {};

          tasks = {
            # Basic after-hook: hook runs after build
            build = lib.mkTask {
              description = "Build the project";
              commands = [ "echo 'Building...'" ];
            };

            post-build = lib.mkTask {
              description = "Post-build hook";
              after = [ "task:build" ];
              commands = [ "echo 'Post-build hook ran'" ];
            };

            # After-hook not triggered: unrelated task should not pull in hooks
            unrelated = lib.mkTask {
              description = "Unrelated task";
              commands = [ "echo 'Unrelated'" ];
            };

            # Transitive after-hooks: c hooks onto b, b hooks onto a
            chain-a = lib.mkTask {
              description = "Chain start";
              commands = [ "echo 'Chain A'" ];
            };

            chain-b = lib.mkTask {
              description = "Chain middle (hooks onto chain-a)";
              after = [ "task:chain-a" ];
              commands = [ "echo 'Chain B'" ];
            };

            chain-c = lib.mkTask {
              description = "Chain end (hooks onto chain-b)";
              after = [ "task:chain-b" ];
              commands = [ "echo 'Chain C'" ];
            };

            # After-hook with its own depends
            setup = lib.mkTask {
              description = "Setup task";
              commands = [ "echo 'Setup'" ];
            };

            deploy = lib.mkTask {
              description = "Deploy task";
              commands = [ "echo 'Deploying...'" ];
            };

            post-deploy = lib.mkTask {
              description = "Post-deploy hook with its own dependency";
              after = [ "task:deploy" ];
              depends = [ "task:setup" ];
              commands = [ "echo 'Post-deploy (needed setup first)'" ];
            };

            # Multiple hooks on the same target
            test = lib.mkTask {
              description = "Run tests";
              commands = [ "echo 'Testing...'" ];
            };

            coverage-report = lib.mkTask {
              description = "Generate coverage report after tests";
              after = [ "task:test" ];
              commands = [ "echo 'Coverage report'" ];
            };

            notify = lib.mkTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
