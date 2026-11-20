{ pkgs, tasksLib }:
let
  inherit (pkgs) lib;
  ntLib = tasksLib.mkLib pkgs;

  eval = modules: tasksLib.evalConfig { inherit pkgs modules; };
  json = modules: builtins.fromJSON (builtins.unsafeDiscardStringContext (builtins.toJSON (eval modules).nixTasksConfig));
  task = name: modules: (json modules).tasks.${name};
  fails = value: !(builtins.tryEval (builtins.toJSON value)).success;
  configFails = modules: fails (eval modules).nixTasksConfig;

  hello = { packages.hello = pkgs.hello; };
  lintBase = { tasks.lint.commands = [ "lint" ]; };

  failures = lib.runTests {
    # Merge semantics

    testCommandsMkForceReplaces = {
      expr = (task "lint" [ lintBase { tasks.lint.commands = lib.mkForce [ "strict" ]; } ]).commands;
      expected = [ "strict" ];
    };
    testCommandsMkBeforeMkAfter = {
      expr = (task "lint" [
        lintBase
        { tasks.lint.commands = lib.mkAfter [ "after" ]; }
        { tasks.lint.commands = lib.mkBefore [ "before" ]; }
      ]).commands;
      expected = [ "before" "lint" "after" ];
    };
    testCommandsTwoPlainDefinitionsConflict = {
      expr = configFails [ lintBase { tasks.lint.commands = [ "other" ]; } ];
      expected = true;
    };
    testCommandsIdenticalDefinitionsMerge = {
      expr = (task "lint" [ lintBase lintBase ]).commands;
      expected = [ "lint" ];
    };
    testCommandsMkDefaultYields = {
      expr = (task "lint" [ { tasks.lint.commands = lib.mkDefault [ "default" ]; } lintBase ]).commands;
      expected = [ "lint" ];
    };
    testScriptMkBeforeMkAfterJoinsLines = {
      expr = (task "s" [
        { tasks.s.script = "body"; }
        { tasks.s.script = lib.mkBefore "pre"; }
        { tasks.s.script = lib.mkAfter "post"; }
      ]).script;
      expected = "pre\nbody\npost";
    };
    testScriptTwoPlainDefinitionsConflict = {
      expr = configFails [ { tasks.s.script = "a"; } { tasks.s.script = "b"; } ];
      expected = true;
    };
    testDepsConcatenateAndDeduplicate = {
      expr = (task "t" [
        { packages = { inherit (pkgs) go hello; }; }
        { tasks.t.deps = [ "go" ]; }
        { tasks.t.deps = [ "hello" "go" ]; }
      ]).deps;
      expected = [ "go" "hello" ];
    };
    testEnvMergesAcrossModules = {
      expr = (task "t" [ { tasks.t.env.A = "1"; } { tasks.t.env.B = "2"; } ]).env;
      expected = { A = "1"; B = "2"; };
    };
    testEnvRejectsNonStrings = {
      expr = configFails [ { tasks.t.env.PORT = 8080; } ];
      expected = true;
    };
    testUnknownTaskFieldRejected = {
      expr = configFails [ { tasks.t.comands = [ "typo" ]; } ];
      expected = true;
    };
    testMkForceNullClearsValue = {
      expr = (task "t" [ { tasks.t.workingDir = "sub"; } { tasks.t.workingDir = lib.mkForce null; } ]) ? workingDir;
      expected = false;
    };
    testDescriptionMkDefaultOverridden = {
      expr = (task "t" [ { tasks.t.description = lib.mkDefault "base"; } { tasks.t.description = "repo"; } ]).description;
      expected = "repo";
    };
    testPackageOverrideReplacesWholeDerivation = {
      expr = ((eval [ { packages.go = pkgs.go; } { packages.go = lib.mkForce pkgs.hello; } ]).config.packages.go) == pkgs.hello;
      expected = true;
    };

    # Assertions

    testAssertUnknownDepends = {
      expr = configFails [ { tasks.t.depends = [ "task:missing" ]; } ];
      expected = true;
    };
    testAssertUnknownAfter = {
      expr = configFails [ { tasks.t.after = [ "task:missing" ]; } ];
      expected = true;
    };
    testAssertSelfDepends = {
      expr = configFails [ { tasks.t.depends = [ "task:t" ]; } ];
      expected = true;
    };
    testAssertSelfAfter = {
      expr = configFails [ { tasks.t.after = [ "t" ]; } ];
      expected = true;
    };
    testAssertUnknownTaskPackage = {
      expr = configFails [ { tasks.t.deps = [ "missing" ]; } ];
      expected = true;
    };
    testAssertUnknownShellPackage = {
      expr = configFails [ { devShells.s.packages = [ "missing" ]; } ];
      expected = true;
    };
    testAssertUnknownExtends = {
      expr = configFails [ { devShells.s.extends = "missing"; } ];
      expected = true;
    };
    testAssertExtendsCycle = {
      expr = configFails [ { devShells = { a.extends = "b"; b.extends = "c"; c.extends = "a"; }; } ];
      expected = true;
    };
    testAssertExtendsSelf = {
      expr = configFails [ { devShells.a.extends = "a"; } ];
      expected = true;
    };
    testAssertCommandsAndScript = {
      expr = configFails [ { tasks.t = { commands = [ "a" ]; script = "b"; }; } ];
      expected = true;
    };
    testAssertBuildTaskWorkingDir = {
      expr = configFails [ { tasks.b = { type = "build"; derivation = pkgs.hello; workingDir = "sub"; }; } ];
      expected = true;
    };
    testAssertBuildTaskCommands = {
      expr = configFails [ { tasks.b = { type = "build"; derivation = pkgs.hello; commands = [ "x" ]; }; } ];
      expected = true;
    };
    testAssertionsGateEveryOutput = {
      expr = map (attr: fails (eval [ { tasks.t.depends = [ "missing" ]; } ]).${attr})
        [ "nixTasksConfig" "nixTasksShellDrvs" "nixTasksBuilds" "devShells" ];
      expected = [ true true true true ];
    };
    testValidConfigPasses = {
      expr = configFails [
        hello
        { tasks.a.commands = [ "a" ]; }
        { tasks.b = { deps = [ "hello" ]; depends = [ "task:a" ]; after = [ "a" ]; commands = [ "b" ]; }; }
        { devShells = { base.packages = [ "hello" ]; top.extends = "base"; }; }
      ];
      expected = false;
    };

    # Builders

    testMkTaskReturnsOnlyGivenFields = {
      expr = ntLib.mkTask { commands = [ "x" ]; };
      expected = { commands = [ "x" ]; };
    };
    testMkGoTask = {
      expr =
        let t = ntLib.mkGoTask { pname = "app"; src = ./.; vendorHash = null; depends = [ "task:gen" ]; };
        in {
          inherit (t) type depends;
          description = t.description.content;
          name = t.derivation.name;
          cgo = t.derivation.CGO_ENABLED;
          ldflags = t.derivation.ldflags;
          hasEnv = t ? env;
        };
      expected = {
        type = "build";
        depends = [ "task:gen" ];
        description = "Build app";
        name = "app-0.1.0";
        cgo = "0";
        ldflags = [ "-buildid=" ];
        hasEnv = false;
      };
    };
    testMkGoTaskComposes = {
      expr = task "app" [
        { tasks.gen.commands = [ "gen" ]; }
        { tasks.app = ntLib.mkGoTask { pname = "app"; src = ./.; vendorHash = null; }; }
        { tasks.app = { description = "Custom"; depends = [ "task:gen" ]; }; }
      ];
      expected = {
        type = "build";
        description = "Custom";
        deps = [ ];
        depends = [ "task:gen" ];
        after = [ ];
        commands = [ ];
        env = { };
        continueOnError = false;
      };
    };
    testMkDockerTask = {
      expr = task "img" [
        { packages.docker = pkgs.hello; }
        { tasks.img = ntLib.mkDockerTask { image = "app"; tag = "v1"; }; }
      ];
      expected = {
        type = "shell";
        description = "";
        deps = [ "docker" ];
        depends = [ ];
        after = [ ];
        commands = [ "docker build -t app:v1 -f Dockerfile ." ];
        env = { };
        continueOnError = false;
      };
    };
    testMkDockerTaskCommandsYieldToOtherModule = {
      expr = (task "img" [
        { packages.docker = pkgs.hello; }
        { tasks.img = ntLib.mkDockerTask { image = "app"; }; }
        { tasks.img.commands = [ "custom" ]; }
      ]).commands;
      expected = [ "custom" ];
    };
    testMkCompoundTask = {
      expr = ntLib.mkCompoundTask { description = "all"; tasks = [ "a" "b" ]; depends = [ "task:c" ]; };
      expected = { description = "all"; depends = [ "task:a" "task:b" "task:c" ]; };
    };
    testMkMakeTask = {
      expr =
        let t = ntLib.compat.make.mkMakeTask { target = "build"; makefile = "other.mk"; makeFlags = [ "V=1" ]; deps = [ "go" ]; };
        in { inherit (t) commands deps; description = t.description.content; };
      expected = { commands = [ "make -f other.mk 'V=1' build" ]; deps = [ "go" ]; description = "Make target: build"; };
    };
    testImportMakeTargets = {
      expr = lib.mapAttrs (_: t: t.commands) (ntLib.compat.make.importMakeTargets { targets = [ "a" "b" ]; prefix = "m"; });
      expected = { m-a = [ "make a" ]; m-b = [ "make b" ]; };
    };

    # Shell inheritance

    testShellInheritance =
      let
        shells = (eval [
          { packages = { inherit (pkgs) go hello jq; }; }
          {
            devShells = {
              base = { packages = [ "go" ]; env = { A = "base"; B = "base"; }; shellHook = "echo base"; };
              mid = { extends = "base"; packages = [ "hello" "go" ]; env.A = "mid"; shellHook = "echo mid"; };
              top = { extends = "mid"; packages = [ "jq" ]; };
              sibling = { extends = "base"; packages = [ "jq" ]; };
            };
          }
        ]).devShells;
        summary = s: {
          packages = map (p: p.pname) s.nativeBuildInputs;
          inherit (s.drvAttrs) A B shellHook;
        };
      in
      {
        expr = { top = summary shells.top; sibling = summary shells.sibling; };
        expected = {
          top = { packages = [ "go" "hello" "jq" ]; A = "mid"; B = "base"; shellHook = "echo base\necho mid"; };
          sibling = { packages = [ "go" "jq" ]; A = "base"; B = "base"; shellHook = "echo base"; };
        };
      };
    testShellEnvCannotReplaceMkShellArgs = {
      expr = fails (eval [ { devShells.s = { shellHook = "real"; env.shellHook = "fake"; }; } ]).devShells.s.drvPath;
      expected = true;
    };

    # Output contract

    testConfigJsonShape =
      let
        cachedTask = { deps = [ "hello" ]; commands = [ "x" ]; cache = { inputs = [ "*.go" ]; outputs = [ ]; }; };
      in
      {
        expr = json [
          hello
          { tasks.cached = cachedTask; }
          { tasks.uncached = { commands = [ "x" ]; }; }
          { tasks.built = { type = "build"; derivation = pkgs.hello; }; }
          { devShells.default = { packages = [ "hello" ]; }; }
          { cacheSettings.maxEntriesPerTask = 16; }
        ];
        expected = {
          packages = [ "hello" ];
          tasks = {
            cached = {
              type = "shell"; description = ""; deps = [ "hello" ]; depends = [ ]; after = [ ];
              commands = [ "x" ]; env = { }; continueOnError = false;
              cache = { inputs = [ "*.go" ]; outputs = [ ]; env = [ ]; };
              shellDrvPath = builtins.unsafeDiscardStringContext ((eval [ hello { tasks.cached = cachedTask; } ]).nixTasksShellDrvs.cached);
            };
            uncached = {
              type = "shell"; description = ""; deps = [ ]; depends = [ ]; after = [ ];
              commands = [ "x" ]; env = { }; continueOnError = false;
            };
            built = {
              type = "build"; description = ""; deps = [ ]; depends = [ ]; after = [ ];
              commands = [ ]; env = { }; continueOnError = false;
            };
          };
          devShells.default = { packages = [ "hello" ]; env = { }; shellHook = ""; };
          cacheSettings = { maxEntriesPerTask = 16; };
        };
      };

    # Cache option

    testCacheListsConcatenateAcrossModules = {
      expr = lib.mapAttrs (_: v: if builtins.isList v then lib.sort lib.lessThan v else v) (task "gen" [
        { tasks.gen = { commands = [ "gen" ]; cache = { inputs = [ "a/**" ]; outputs = [ "out" ]; env = [ "A" ]; }; }; }
        { tasks.gen.cache = { inputs = [ "b/**" ]; outputs = [ "out2" ]; env = [ "B" ]; maxEntries = 3; }; }
      ]).cache;
      expected = { inputs = [ "a/**" "b/**" ]; outputs = [ "out" "out2" ]; env = [ "A" "B" ]; maxEntries = 3; };
    };
    testCacheMkForceNullDisables = {
      expr = (task "gen" [
        { tasks.gen = { commands = [ "gen" ]; cache = { inputs = [ "a" ]; outputs = [ ]; }; }; }
        { tasks.gen.cache = lib.mkForce null; }
      ]) ? cache;
      expected = false;
    };
    testCacheSettingsDefaultEmpty = {
      expr = (json [ ]).cacheSettings;
      expected = { };
    };
    testCacheRequiresOutputs = {
      expr = configFails [ { tasks.gen = { commands = [ "gen" ]; cache.inputs = [ "a" ]; }; } ];
      expected = true;
    };
    testCacheRequiresInputs = {
      expr = configFails [ { tasks.gen = { commands = [ "gen" ]; cache = { inputs = [ ]; outputs = [ ]; }; }; } ];
      expected = true;
    };
    testCacheRejectedOnBuildTask = {
      expr = configFails [ { tasks.b = { type = "build"; derivation = pkgs.hello; cache = { inputs = [ "a" ]; outputs = [ ]; }; }; } ];
      expected = true;
    };
    testCacheRejectedOnTaskWithoutCommands = {
      expr = configFails [ { tasks.all = { depends = [ ]; cache = { inputs = [ "a" ]; outputs = [ ]; }; }; } ];
      expected = true;
    };
    testCacheMaxEntriesMustBePositive = {
      expr = configFails [ { tasks.gen = { commands = [ "gen" ]; cache = { inputs = [ "a" ]; outputs = [ ]; maxEntries = 0; }; }; } ];
      expected = true;
    };
    testRemovedInputsFieldNamesReplacement = {
      expr = builtins.tryEval (builtins.toJSON (eval [ { tasks.t = ntLib.mkTask { commands = [ "x" ]; inputs = [ "*.go" ]; }; } ]).nixTasksConfig);
      expected = { success = false; value = false; };
    };
    testRemovedOutputsFieldRejected = {
      expr = configFails [ { tasks.t = { commands = [ "x" ]; outputs = [ "out" ]; }; } ];
      expected = true;
    };
    testNoCacheIgnored = {
      expr = task "t" [ { tasks.t = { commands = [ "x" ]; noCache = true; }; } ] ? noCache;
      expected = false;
    };

    testConfigDoesNotForceBuildDerivations = {
      expr = configFails [ { tasks.b = { type = "build"; derivation = throw "forced"; }; } ];
      expected = false;
    };
    testLabelSerialized = {
      expr = (task "t" [ { tasks.t = { label = "short"; commands = [ "x" ]; }; } ]).label;
      expected = "short";
    };
    testLabelOmittedWhenUnset = {
      expr = (task "t" [ { tasks.t.commands = [ "x" ]; } ]) ? label;
      expected = false;
    };
    testLabelRejectsEmpty = {
      expr = configFails [ { tasks.t.label = ""; } ];
      expected = true;
    };
    testMkGoTaskKeepsLabel = {
      expr = (ntLib.mkGoTask { pname = "app"; src = ./.; vendorHash = null; label = "app"; }).label;
      expected = "app";
    };
    testConfigDoesNotForcePackages = {
      expr = (json [ { packages.broken = throw "forced"; } ]).packages;
      expected = [ "broken" ];
    };
    testBuildsOnlyContainBuildTasks = {
      expr = builtins.attrNames (eval [
        { tasks.s.commands = [ "x" ]; }
        { tasks.b = { type = "build"; derivation = pkgs.hello; }; }
      ]).nixTasksBuilds;
      expected = [ "b" ];
    };
    testShellsOnlyContainShellTasks = {
      expr = builtins.attrNames (eval [
        { tasks.s.commands = [ "x" ]; }
        { tasks.b = { type = "build"; derivation = pkgs.hello; }; }
      ]).nixTasksShellDrvs;
      expected = [ "s" ];
    };
    testMkFlakeOutputsShape = {
      expr = lib.mapAttrs (_: builtins.attrNames) (tasksLib.mkFlakeOutputs {
        nixpkgs = { legacyPackages.${pkgs.stdenv.hostPlatform.system} = pkgs; };
        systems = [ pkgs.stdenv.hostPlatform.system ];
        modules = [ ];
      });
      expected = lib.genAttrs
        [ "nixTasksConfig" "nixTasksShellDrvs" "nixTasksBuilds" "devShells" ]
        (_: [ pkgs.stdenv.hostPlatform.system ]);
    };
  };
in
if failures == [ ] then
  pkgs.runCommand "nix-tasks-lib-tests" { } "touch $out"
else
  throw "nix-tasks lib tests failed:\n${lib.generators.toPretty { } failures}"
