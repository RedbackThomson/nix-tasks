{ lib }:
let
  # Merging two plain definitions of a task's commands would silently run
  # both (base `go test` and repo `go test -race`), so only one definition may
  # omit an explicit order. The module system keeps `priority` on definitions
  # that came from mkBefore/mkAfter/mkOrder after sorting them.
  orderedOnce = { name, description, check, merge }:
    lib.mkOptionType {
      inherit name description check;
      merge = loc: defs:
        let
          plain = builtins.filter (def: !(def ? priority)) defs;
          distinct = lib.unique (map (def: def.value) plain);
          kept = lib.foldl' (acc: def:
            if def ? priority || !acc.seenPlain then
              { seenPlain = acc.seenPlain || !(def ? priority); defs = acc.defs ++ [ def ]; }
            else acc
          ) { seenPlain = false; defs = [ ]; } defs;
        in
        if builtins.length distinct > 1 then
          throw ''
            The option `${lib.showOption loc}' is defined in more than one module without an explicit order:
            ${lib.concatMapStrings (def: "  - ${def.file}\n") plain}Definitions are not combined implicitly, because that would run every module's commands.
            Use lib.mkForce to replace the other definitions, or lib.mkBefore / lib.mkAfter to add to them.''
        else
          merge (map (def: def.value) kept.defs);
    };
in
{
  commands = orderedOnce {
    name = "commands";
    description = "list of shell commands, defined once or combined with mkBefore/mkAfter";
    check = v: builtins.isList v && builtins.all builtins.isString v;
    merge = builtins.concatLists;
  };

  script = orderedOnce {
    name = "script";
    description = "shell script, defined once or combined with mkBefore/mkAfter";
    check = builtins.isString;
    merge = lib.concatStringsSep "\n";
  };
}
