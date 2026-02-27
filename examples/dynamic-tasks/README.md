# Dynamic Tasks Example

This example demonstrates how to generate tasks dynamically from a data list using Nix's `map` and `builtins.listToAttrs`. It models a monorepo with multiple microservices where each service gets its own build and deploy tasks generated automatically.

## Task Graph

```
deploy-all
├── deploy-api-gateway
│   └── build-api-gateway
│       └── generate-protos
├── deploy-order-service
│   └── build-order-service
│       └── generate-protos
└── deploy-user-service
    └── build-user-service
        └── generate-protos
```

`generate-protos` is a shared dependency — nix-tasks only runs it once even though all three build tasks depend on it.

## How It Works

A `services` list drives all task generation:

```nix
services = [
  { name = "api-gateway";  port = 8080; path = "cmd/api-gateway"; }
  { name = "user-service"; port = 8081; path = "cmd/user-service"; }
  { name = "order-service"; port = 8082; path = "cmd/order-service"; }
];
```

Adding or removing an entry from this list automatically updates every generated task and the aggregate `build-all` / `deploy-all` targets.

### Per-service tasks (`builtins.listToAttrs` + `map`)

```nix
buildTasks = builtins.listToAttrs (map (svc: {
  name = "build-${svc.name}";
  value = lib.mkTask {
    description = "Build ${svc.name}";
    deps = [ "go" ];
    depends = [ "task:generate-protos" ];
    commands = [ "echo 'Building ${svc.name}...'" ];
  };
}) services);
```

### Aggregate fan-in tasks

```nix
build-all = lib.mkTask {
  description = "Build all services";
  depends = map (svc: "task:build-${svc.name}") services;
  commands = [ "echo 'All services built.'" ];
};
```

## Running

```bash
# List all generated tasks
nix-tasks list

# Build everything (runs generate-protos once, then all builds in parallel)
nix-tasks run build-all

# Deploy everything (builds first, then deploys in parallel)
nix-tasks run deploy-all

# Build/deploy a single service
nix-tasks run build-api-gateway
nix-tasks run deploy-user-service
```

## Key Patterns

| Pattern | Technique |
|---------|-----------|
| Generate N tasks from a list | `builtins.listToAttrs` + `map` |
| Fan-in aggregate task | `depends = map (svc: "task:build-${svc.name}") services` |
| Shared dependency (run once) | Multiple tasks `depends` on the same task |
| Merge task sets | `sharedTasks // buildTasks // deployTasks // aggregateTasks` |
| Graceful empty list | `if services == [] then [] else ...` |
