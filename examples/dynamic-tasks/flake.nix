{
  description = "nix-tasks example - dynamically generated tasks from a list";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-tasks.url = "path:../..";
  };

  outputs = { self, nixpkgs, nix-tasks }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # ── Service definitions ──────────────────────────────────────────
      # Each service represents a microservice that gets its own build,
      # test, and deploy tasks generated automatically.
      services = [
        { name = "api-gateway";  port = 8080; path = "cmd/api-gateway"; }
        { name = "user-service"; port = 8081; path = "cmd/user-service"; }
        { name = "order-service"; port = 8082; path = "cmd/order-service"; }
      ];

      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};

          # ── Shared dependency task ─────────────────────────────────
          # All per-service build tasks depend on this. nix-tasks will
          # only run it once even when multiple services reference it.
          sharedTasks = {
            generate-protos = lib.mkTask {
              description = "Generate protobuf stubs for all services";
              commands = [
                "echo 'Generating protobuf definitions...'"
                "echo 'proto/user.proto   -> pkg/pb/user.pb.go'"
                "echo 'proto/order.proto  -> pkg/pb/order.pb.go'"
                "echo 'proto/common.proto -> pkg/pb/common.pb.go'"
                "echo 'Protobuf generation complete'"
              ];
            };
          };

          # ── Dynamically generated per-service tasks ────────────────
          buildTasks = builtins.listToAttrs (map (svc: {
            name = "build-${svc.name}";
            value = lib.mkTask {
              description = "Build ${svc.name}";
              deps = [ "go" ];
              depends = [ "task:generate-protos" ];
              commands = [
                "echo 'Building ${svc.name} from ${svc.path}...'"
                "echo 'Compiling ${svc.path}/main.go'"
                "echo 'Built: bin/${svc.name}'"
              ];
            };
          }) services);

          deployTasks = builtins.listToAttrs (map (svc: {
            name = "deploy-${svc.name}";
            value = lib.mkTask {
              description = "Deploy ${svc.name} to Kubernetes";
              depends = [ "task:build-${svc.name}" ];
              noCache = true;
              commands = [
                "echo 'Deploying ${svc.name}...'"
                "echo 'Applying k8s manifest for ${svc.name} (port ${toString svc.port})'"
                "echo 'Waiting for rollout...'"
                "echo '${svc.name} deployed successfully'"
              ];
            };
          }) services);

          # ── Fan-in aggregate tasks ─────────────────────────────────
          aggregateTasks = {
            build-all = lib.mkTask {
              description = "Build all services";
              depends =
                if services == [] then []
                else map (svc: "task:build-${svc.name}") services;
              commands =
                if services == [] then [ "echo 'No services configured.'" ]
                else [ "echo 'All ${toString (builtins.length services)} services built successfully.'" ];
            };

            deploy-all = lib.mkTask {
              description = "Deploy all services to Kubernetes";
              depends =
                if services == [] then []
                else map (svc: "task:deploy-${svc.name}") services;
              noCache = true;
              commands =
                if services == [] then [ "echo 'No services configured.'" ]
                else [ "echo 'All ${toString (builtins.length services)} services deployed.'" ];
            };
          };

        in lib.evalConfig {
          packages = {
            go = pkgs.go;
          };

          tasks = sharedTasks // buildTasks // deployTasks // aggregateTasks;

          devShells = {
            default = {
              packages = [ "go" ];
              shellHook = ''
                echo "Dynamic tasks example"
                echo ""
                echo "Services: ${builtins.concatStringsSep ", " (map (s: s.name) services)}"
                echo ""
                echo "Try: nix-tasks list"
                echo ""
              '';
            };
          };
        };
    in {
      # Expose nix-tasks CLI as a package and app
      packages = forAllSystems (system: {
        nix-tasks = nix-tasks.packages.${system}.default;
        default = nix-tasks.packages.${system}.default;
      });

      apps = forAllSystems (system: {
        nix-tasks = nix-tasks.apps.${system}.default;
        default = nix-tasks.apps.${system}.default;
      });

      # Expose task configuration for nix-tasks CLI
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);

      # Expose dev shells with nix-tasks available
      devShells = forAllSystems (system:
        let
          config = mkConfig system;
          pkgs = nixpkgs.legacyPackages.${system};
        in
        config.devShells // {
          default = config.devShells.default.overrideAttrs (old: {
            buildInputs = (old.buildInputs or []) ++ [ nix-tasks.packages.${system}.default ];
          });
        }
      );
    };
}
