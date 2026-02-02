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
      {
        packages.default = pkgs.buildGoModule {
          pname = "nix-tasks";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # Update after first build with dependencies
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_22
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
