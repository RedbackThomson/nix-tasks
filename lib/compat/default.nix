# Compatibility layer for nix-tasks
#
# This module provides helpers for migrating from other build systems
# to nix-tasks.
#
# Currently supported:
#   - Make (Makefile)
#
# Usage:
#   { tasksLib, ... }: {
#     tasks.legacy = tasksLib.compat.make.mkMakeTask { target = "build"; };
#   }
#
{ lib }:
{
  make = import ./make.nix { inherit lib; };
}
