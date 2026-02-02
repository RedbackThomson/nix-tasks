{
  description = "Simple nix-tasks example";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      system = "aarch64-darwin"; # Change to your system
      pkgs = nixpkgs.legacyPackages.${system};
      lib = nix-tasks.lib.${system};

      config = lib.evalConfig {
        packages = {
          go = pkgs.go;
          docker = pkgs.docker;
        };

        tasks = {
          build = lib.mkGoTask {
            description = "Build the application";
            output = "bin/app";
          };

          test = lib.mkTask {
            description = "Run tests";
            deps = ["go"];
            commands = ["go test ./..."];
          };

          all = lib.mkCompoundTask {
            description = "Run build and test";
            tasks = ["build" "test"];
          };
        };

        devShells = {
          default = {
            packages = ["go" "docker"];
            shellHook = ''
              echo "Development shell ready"
            '';
          };
        };
      };
    in config // {
      devShells.${system} = config.devShells;
    };
}
