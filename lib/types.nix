{ lib }:
{
  # Task type definition
  taskType = {
    description ? "",
    deps ? [],
    depends ? [],
    commands ? [],
    script ? null,
    env ? {},
    workingDir ? null,
    inputs ? [],
    outputs ? [],
    continueOnError ? false,
  }: {
    inherit description deps depends commands script env workingDir inputs outputs continueOnError;
  };

  # Shell type definition
  shellType = {
    extends ? null,
    packages ? [],
    env ? {},
    shellHook ? "",
  }: {
    inherit extends packages env shellHook;
  };

  # Validate configuration
  validateConfig = config:
    let
      # Ensure required fields exist
      packages = config.packages or {};
      tasks = config.tasks or {};
      devShells = config.devShells or {};
    in {
      inherit packages tasks devShells;
    };
}
