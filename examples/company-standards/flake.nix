{
  description = "Company-wide nix-tasks standards - example template";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
    nix-tasks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [ self.nixTasksModules.default ];
    } // {
      # Repositories add this to their own `modules`.
      nixTasksModules.default = ./standards.nix;

      inherit (nix-tasks) packages apps;
    };
}
