# Compatibility layer for nix-tasks
#
# This module provides helpers for migrating from other build systems
# to nix-tasks.
#
# Currently supported:
#   - Make (Makefile)
#
# Usage:
#   compat = nix-tasks.lib.compat;
#   tasks = {
#     legacy = compat.make.mkMakeTask { target = "build"; };
#   };
#
{ lib, pkgs, builders }:
{
  # Make compatibility helpers
  make = import ./make.nix { inherit lib pkgs builders; };
}
