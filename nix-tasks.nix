{ pkgs, tasksLib, ... }:
{
  packages = {
    go = pkgs.go;
    golangci-lint = pkgs.golangci-lint;
  };

  tasks = {
    build = tasksLib.mkGoTask {
      pname = "nix-tasks";
      version = "0.1.0";
      description = "Build nix-tasks binary";
      src = ./.;
      vendorHash = "sha256-CvMBl54ppkQP3OTL7hXk6JlZKsQTkZCMqSZNwKMqHG8=";
      ldflags = [ "-s" "-w" ];
      nativeBuildInputs = [ pkgs.installShellFiles ];
      postInstall = ''
        installShellCompletion --cmd nix-tasks \
          --bash <($out/bin/nix-tasks completions bash) \
          --zsh <($out/bin/nix-tasks completions zsh) \
          --fish <($out/bin/nix-tasks completions fish)
      '';
    };

    test = tasksLib.mkTask {
      description = "Run Go tests";
      deps = [ "go" ];
      commands = [ "go test -v ./..." ];
    };

    lint = tasksLib.mkTask {
      description = "Run golangci-lint";
      deps = [ "golangci-lint" ];
      commands = [ "golangci-lint run ./..." ];
    };

    fmt = tasksLib.mkTask {
      description = "Format Go code";
      deps = [ "go" ];
      commands = [ "go fmt ./..." ];
    };

    tidy = tasksLib.mkTask {
      description = "Tidy Go dependencies";
      deps = [ "go" ];
      commands = [ "go mod tidy" ];
    };

    clean = tasksLib.mkTask {
      description = "Clean build artifacts";
      commands = [
        "rm -rf result nix-tasks"
        "echo 'Cleaned build artifacts'"
      ];
    };

    check = tasksLib.mkCompoundTask {
      description = "Run all checks";
      tasks = [ "test" "lint" ];
    };
  };

  # The shell deliberately omits the nix-tasks package: its vendorHash goes
  # stale whenever go.mod changes, which would lock developers out of the shell.
  devShells.default = {
    packages = [ "go" "golangci-lint" ];
    shellHook = ''
      echo "nix-tasks development shell"
      echo "Run the CLI from source with: go run ./cmd/nix-tasks"
    '';
  };
}
