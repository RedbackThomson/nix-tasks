# Walks a shell's `extends` chain eagerly: a lazy recursive merge never
# reaches its own cycle check and overflows the stack instead.
{ lib }:
shells: name:
let
  walk = visited: current:
    if builtins.elem current visited then
      { chain = visited; cycle = visited ++ [ current ]; }
    else
      let
        parent = shells.${current}.extends or null;
        visited' = visited ++ [ current ];
      in
      if parent == null || !(shells ? ${parent}) then
        { chain = visited'; cycle = null; }
      else
        walk visited' parent;
in
walk [ ] name
