{
  description = "Test mkGoTask builder";

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
          packages = {
            go = pkgs.go;
          };

          tasks = {
            # Test mkGoTask with minimal configuration (build task)
            build = lib.mkGoTask {
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
            test = lib.mkShellTask {
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
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
