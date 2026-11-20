{
  description = "nix-tasks example - a pipeline for checking the GitHub Actions output";

  inputs = {
    nix-tasks.url = "path:../..";
    nixpkgs.follows = "nix-tasks/nixpkgs";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    nix-tasks.lib.mkFlakeOutputs {
      inherit nixpkgs;
      modules = [
        ({ tasksLib, ... }: {
          tasks = {
            generate = tasksLib.mkTask {
              description = "Print a line a second, interleaved with other tasks";
              commands = [ "for i in 1 2 3 4 5; do echo \"generating file $i\"; sleep 1; done" ];
            };

            generate-crossplane-provider-aws-ec2-managed-resources-crds = tasksLib.mkTask {
              description = "A long name with a label";
              label = "ec2-crds";
              commands = [ "for i in 1 2 3; do echo \"wrote crd $i\"; sleep 1; done" ];
            };

            test-unit-crossplane-provider-aws-ec2-managed-resources = tasksLib.mkTask {
              description = "A long name without a label, shown abbreviated";
              commands = [ "for i in 1 2 3 4; do echo \"=== RUN TestReconcile/$i\"; sleep 1; done" ];
            };

            lint = tasksLib.mkTask {
              description = "Fails when FAIL_LINT=true; prints colour and workflow commands";
              commands = [
                "printf '\\033[1mapis/v1/types.go:42:2\\033[0m: ineffectual assignment to err\\n'"
                "echo '::warning file=apis/v1/types.go,line=42,col=2::ineffectual assignment to err'"
                "echo '::group::Lint details'"
                "echo 'checked 12 files'"
                "echo '::endgroup::'"
                "echo '::add-mask::example-token'"
                "echo 'token: example-token'"
                "echo '::endgroup::'"
                "sleep 2"
                "if [ \"\${FAIL_LINT:-}\" = true ]; then exit 1; fi"
              ];
            };

            fmt-check = tasksLib.mkTask {
              description = "Always fails, but is allowed to";
              continueOnError = true;
              commands = [ "echo 'main.go is not formatted'" "exit 1" ];
            };

            build = tasksLib.mkTask {
              description = "Build after generating";
              depends = [ "task:generate" "task:generate-crossplane-provider-aws-ec2-managed-resources-crds" ];
              commands = [ "echo 'go build ./...'" "sleep 1" ];
            };

            deploy = tasksLib.mkTask {
              description = "Skipped when lint fails";
              depends = [ "task:build" "task:lint" "task:test-unit-crossplane-provider-aws-ec2-managed-resources" "task:fmt-check" ];
              commands = [ "echo 'deploying'" ];
            };
          };
        })
      ];
    };
}
