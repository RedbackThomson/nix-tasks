{ lib }:
let
  defaultSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

  mkLib = pkgs:
    import ./builders.nix { inherit pkgs; inherit (pkgs) lib; }
    // { compat = import ./compat { inherit (pkgs) lib; }; };

  evalConfig = import ./eval.nix { inherit mkLib; };

  outputAttrs = [ "nixTasksConfig" "nixTasksShellDrvs" "nixTasksBuilds" "devShells" ];

  mkFlakeOutputs = {
    nixpkgs,
    modules,
    systems ? defaultSystems,
    specialArgs ? { },
  }:
    let
      perSystem = lib.genAttrs systems (system: evalConfig {
        pkgs = nixpkgs.legacyPackages.${system};
        inherit modules specialArgs;
      });
    in
    lib.genAttrs outputAttrs (attr: lib.mapAttrs (_: result: result.${attr}) perSystem);
in
{
  inherit mkLib evalConfig mkFlakeOutputs defaultSystems;
}
