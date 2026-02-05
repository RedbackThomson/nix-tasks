# Installation

## Using with nix-shell

Run `nix-tasks` without installing:

```bash
nix shell github:redbackthomson/nix-tasks
nix-tasks --help
```

Or from a local checkout:

```bash
nix shell .#nix-tasks
nix-tasks --help
```

## Using with nix run

Run `nix-tasks` directly without entering a shell:

```bash
nix run github:redbackthomson/nix-tasks -- list
nix run github:redbackthomson/nix-tasks -- run build
```

Or from a local checkout:

```bash
nix run . -- list
nix run . -- run build
```

## Installing in home-manager

Add to your `home.nix` or `home-manager` configuration:

```nix
{ config, pkgs, ... }:

{
  home.packages = [
    # From GitHub
    inputs.nix-tasks.packages.${pkgs.system}.nix-tasks

    # Or use the default package
    inputs.nix-tasks.packages.${pkgs.system}.default
  ];
}
```

Add the input to your `flake.nix`:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager.url = "github:nix-community/home-manager";
    nix-tasks.url = "github:redbackthomson/nix-tasks";
  };

  outputs = { nixpkgs, home-manager, nix-tasks, ... }: {
    homeConfigurations.yourname = home-manager.lib.homeManagerConfiguration {
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
      modules = [
        ({ pkgs, ... }: {
          home.packages = [
            nix-tasks.packages.${pkgs.system}.nix-tasks
          ];
        })
      ];
    };
  };
}
```

## Installing in NixOS

Add to your `configuration.nix`:

```nix
{ config, pkgs, ... }:

{
  environment.systemPackages = [
    inputs.nix-tasks.packages.${pkgs.system}.nix-tasks
  ];
}
```

## Installing with nix profile

Install persistently using Nix profiles:

```bash
# From GitHub
nix profile install github:redbackthomson/nix-tasks

# From local checkout
nix profile install .#nix-tasks

# Verify installation
nix-tasks --help
```

## Development

Enter the development shell with all required tools:

```bash
nix develop
```

Build from source:

```bash
nix build .#nix-tasks
./result/bin/nix-tasks --help
```
