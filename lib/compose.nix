# Configuration composition utilities for nix-tasks
#
# This module provides helpers for extending and customizing configurations,
# enabling company-wide standards with per-repository overrides.
#
# Usage:
#   inherit (compose) override append prepend extend;
#
#   # Override a value completely
#   tasks.lint.commands = override ["custom-lint ./..."];
#
#   # Append to a list
#   packages = append ["extra-tool"];
#
#   # Prepend to a list
#   tasks.test.commands = prepend ["echo 'Running custom pre-test'"];
#
#   # Extend a base configuration
#   config = extend baseConfig { tasks.myTask = ...; };
#
{ lib }:
rec {
  # Override marker - completely replaces the value during merge
  #
  # Example:
  #   tasks.lint.commands = override ["my-custom-lint ./..."];
  #
  override = value: {
    __override = true;
    __value = value;
  };

  # Append to list marker - appends values to existing list during merge
  #
  # Example:
  #   tasks.build.deps = append ["extra-dep"];
  #
  append = values: {
    __append = true;
    __values = values;
  };

  # Prepend to list marker - prepends values to existing list during merge
  #
  # Example:
  #   tasks.test.commands = prepend ["echo 'Pre-test hook'"];
  #
  prepend = values: {
    __prepend = true;
    __values = values;
  };

  # Check if a value is an override marker
  isOverrideMarker = value:
    builtins.isAttrs value &&
    (value ? __override || value ? __append || value ? __prepend);

  # Process a single value during merge, handling override markers
  #
  # Arguments:
  #   base    - The base value being merged into
  #   overlay - The overlay value (may contain override markers)
  #
  # Returns the merged result
  processValue = base: overlay:
    if !builtins.isAttrs overlay then
      overlay
    else if overlay ? __override then
      overlay.__value
    else if overlay ? __append then
      if builtins.isList base then
        base ++ overlay.__values
      else
        overlay.__values
    else if overlay ? __prepend then
      if builtins.isList base then
        overlay.__values ++ base
      else
        overlay.__values
    else if builtins.isAttrs base && !isOverrideMarker base then
      mergeAttrs base overlay
    else
      overlay;

  # Deep merge two attribute sets, processing override markers
  #
  # Arguments:
  #   base    - The base attribute set
  #   overlay - The overlay attribute set with optional override markers
  #
  # Returns a new attribute set with deep-merged values
  #
  # Example:
  #   mergeAttrs
  #     { tasks.build.deps = ["go"]; tasks.build.commands = ["go build"]; }
  #     { tasks.build.deps = append ["docker"]; }
  #   => { tasks.build.deps = ["go" "docker"]; tasks.build.commands = ["go build"]; }
  #
  mergeAttrs = base: overlay:
    let
      baseKeys = builtins.attrNames base;
      overlayKeys = builtins.attrNames overlay;
      allKeys = lib.unique (baseKeys ++ overlayKeys);
    in
    builtins.listToAttrs (map (key:
      let
        baseVal = base.${key} or null;
        overlayVal = overlay.${key} or null;
        merged =
          if overlayVal == null then
            baseVal
          else if baseVal == null then
            # If overlay has a marker but no base value, handle appropriately
            if builtins.isAttrs overlayVal && overlayVal ? __override then
              overlayVal.__value
            else if builtins.isAttrs overlayVal && overlayVal ? __append then
              overlayVal.__values
            else if builtins.isAttrs overlayVal && overlayVal ? __prepend then
              overlayVal.__values
            else
              overlayVal
          else
            processValue baseVal overlayVal;
      in
      { name = key; value = merged; }
    ) allKeys);

  # Extend a base configuration with an overlay
  #
  # The overlay can be either:
  #   - An attribute set to merge
  #   - A function that receives the base and returns an attribute set
  #
  # Arguments:
  #   base    - The base configuration to extend
  #   overlay - The overlay (attribute set or function)
  #
  # Returns the extended configuration
  #
  # Example with attribute set:
  #   extend baseConfig {
  #     tasks.myTask = lib.mkTask { ... };
  #     tasks.lint.commands = override ["custom-lint"];
  #   }
  #
  # Example with function (for accessing base values):
  #   extend baseConfig (base: {
  #     packages = base.packages // { myPkg = pkgs.myPkg; };
  #     tasks.myTask = lib.mkTask { deps = base.packages.go; ... };
  #   })
  #
  extend = base: overlay:
    let
      overlayResolved =
        if builtins.isFunction overlay then
          overlay base
        else
          overlay;
    in
    mergeAttrs base overlayResolved;

  # Create a composable configuration module
  #
  # This is useful for creating reusable configuration fragments that can be
  # combined together.
  #
  # Arguments:
  #   config - The configuration fragment
  #
  # Returns a module that can be used with composeConfigs
  mkModule = config: {
    __isModule = true;
    config = config;
  };

  # Compose multiple configuration modules together
  #
  # Arguments:
  #   modules - List of modules created with mkModule
  #
  # Returns the composed configuration
  composeConfigs = modules:
    builtins.foldl' (acc: mod:
      let
        cfg = if mod ? __isModule then mod.config else mod;
      in
      extend acc cfg
    ) {} modules;
}
