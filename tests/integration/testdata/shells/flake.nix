{
  description = "Shells test fixture - dev shells with inheritance";

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
            jq = pkgs.jq;
            curl = pkgs.curl;
          };

          tasks = {
            check-jq = lib.mkTask {
              description = "Check jq is available";
              deps = [ "jq" ];
              commands = [ "jq --version" ];
            };

            check-curl = lib.mkTask {
              description = "Check curl is available";
              deps = [ "curl" ];
              commands = [ "curl --version | head -1" ];
            };
          };

          devShells = {
            minimal = {
              packages = [ "jq" ];
              env = {
                SHELL_TYPE = "minimal";
              };
            };

            extended = {
              extends = "minimal";
              packages = [ "curl" ];
              env = {
                SHELL_TYPE = "extended";
              };
            };

            default = {
              extends = "extended";
              shellHook = ''
                echo "Welcome to the test shell"
              '';
            };
          };
        };
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
