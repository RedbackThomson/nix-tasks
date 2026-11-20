# Module body for the cache tests, which copy it into a temporary flake so
# they can edit inputs and outputs. Every task appends its name to runs.log,
# so tests can count real executions.
{
  tasks = {
    generate = tasksLib.mkTask {
      description = "Concatenate src/*.txt into gen/out.txt";
      commands = [
        "mkdir -p gen"
        "cat src/*.txt > gen/out.txt"
        "echo generate >> runs.log"
      ];
      cache = {
        inputs = [ "src/*.txt" ];
        outputs = [ "gen/out.txt" ];
      };
    };

    package = tasksLib.mkTask {
      description = "Copy the generated file";
      depends = [ "task:generate" ];
      commands = [
        "cp gen/out.txt package.txt"
        "echo package >> runs.log"
      ];
      cache = {
        inputs = [ "gen/out.txt" ];
        outputs = [ "package.txt" ];
      };
    };

    fail = tasksLib.mkTask {
      commands = [
        "echo fail >> runs.log"
        "exit 1"
      ];
      cache = {
        inputs = [ "src/*.txt" ];
        outputs = [ ];
      };
    };

    # Blocks while the file `block` exists, so a test can interrupt it.
    slow = tasksLib.mkTask {
      commands = [
        "echo slow >> runs.log"
        "if [ -e block ]; then echo started > slow.started; sleep 60; fi"
      ];
      cache = {
        inputs = [ "src/*.txt" ];
        outputs = [ ];
      };
    };

    # A build task: cached tasks downstream of it hit while its derivation,
    # which tool.txt feeds, is unchanged.
    tool = {
      type = "build";
      derivation = pkgs.writeText "tool" (builtins.readFile ./tool.txt);
    };

    use-tool = tasksLib.mkTask {
      depends = [ "task:tool" ];
      commands = [ "echo use-tool >> runs.log" ];
      cache = {
        inputs = [ "src/*.txt" ];
        outputs = [ ];
      };
    };

    tools = tasksLib.mkTask {
      commands = [ "echo tools >> runs.log" ];
    };

    downstream = tasksLib.mkTask {
      depends = [ "task:tools" ];
      commands = [ "echo downstream >> runs.log" ];
      cache = {
        inputs = [ "src/*.txt" ];
        outputs = [ ];
      };
    };
  };
}
