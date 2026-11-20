{
  description = "Test mkGoTask builder";

  inputs = {
    nix-tasks.url = "path:../../../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ pkgs, tasksLib, ... }: {
          packages = {
            go = pkgs.go;
          };

          tasks = {
            # Test mkGoTask with minimal configuration (build task)
            build = tasksLib.mkGoTask {
              description = "Build Go application using buildGoModule";
              pname = "testapp";
              version = "1.0.0";
              src = ./src;
              # For vendored dependencies, use null or omit
              # For modules, calculate with: nix-prefetch '{ sha256 }: (callPackage ./. { }).goModules.overrideAttrs (_: { modSha256 = sha256; })'
              vendorHash = null;  # No external dependencies
              # Outputs are automatically linked to .nix-tasks/build/out/bin/testapp
            };

            # Test running the built binary from .nix-tasks
            test = tasksLib.mkShellTask {
              description = "Test the built binary";
              depends = [ "task:build" ];
              commands = [
                ".nix-tasks/build/out/bin/testapp"
                "test -f .nix-tasks/build/out/bin/testapp || (echo 'Binary not found' && exit 1)"
              ];
            };
          };

          devShells = {
            default = {
              packages = [ "go" ];
            };
          };
        })
      ];
    };
}
