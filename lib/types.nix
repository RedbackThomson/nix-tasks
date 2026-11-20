{ lib }:
{
  validateConfig = config:
    let
      packages = config.packages or {};
      tasks = config.tasks or {};
      devShells = config.devShells or {};
    in {
      inherit packages tasks devShells;
    };
}
