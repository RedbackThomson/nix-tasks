{
  description = "Nix-based task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = fn: nixpkgs.lib.genAttrs systems (system: fn system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
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
          default = nix-tasks;
          nix-tasks = nix-tasks;
        }
      );

      apps = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          nix-tasks = self.packages.${system}.nix-tasks;
        in
        {
          default = {
            type = "app";
            program = "${nix-tasks}/bin/nix-tasks";
          };
        }
      );

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              golangci-lint
              nix
            ];
          };
        }
      );

      lib = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        import ./lib { inherit pkgs; lib = pkgs.lib; }
      );
    };
}
