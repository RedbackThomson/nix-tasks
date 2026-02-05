{
  description = "Nix-based task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      let
        nix-tasks = pkgs.buildGoModule {
          pname = "nix-tasks";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-48Va8grh1BHGxZgwHKWvsB+HvBmmxD78cEDWl1Ysn4Y=";

          meta = with pkgs.lib; {
            description = "Nix-based task runner and development environment manager";
            homepage = "https://github.com/redbackthomson/nix-tasks";
            license = licenses.mit;
            mainProgram = "nix-tasks";
          };
        };
      in
      {
        packages = {
          default = nix-tasks;
          nix-tasks = nix-tasks;
        };

        apps.default = {
          type = "app";
          program = "${nix-tasks}/bin/nix-tasks";
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            golangci-lint
            nix
          ];
        };

        # Export the library for consumers
        lib = import ./lib { inherit pkgs; lib = pkgs.lib; };
      }
    );
}
