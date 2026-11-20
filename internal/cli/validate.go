package cli

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// ValidateCmd validates the configuration
type ValidateCmd struct{}

// buildChecker reports build tasks whose derivation cannot be found.
type buildChecker interface {
	UnbuildableTasks(ctx context.Context, system string, tasks []string) ([]string, error)
}

// Run executes the validate command
func (c *ValidateCmd) Run(globals *Globals) error {
	ctx := context.Background()

	cfg, eval, err := loadConfig(ctx, globals)
	if err != nil {
		return err
	}

	if err := validateConfig(ctx, cfg, eval); err != nil {
		return err
	}

	for _, w := range config.CacheWarnings(cfg) {
		warn("%s", w)
	}

	fmt.Printf("%s Configuration is valid\n", ui.Success("✓"))
	fmt.Printf("  Tasks: %d\n", len(cfg.Tasks))
	fmt.Printf("  Packages: %d\n", len(cfg.Packages))
	fmt.Printf("  Dev Shells: %d\n", len(cfg.DevShells))
	for _, w := range config.LabelWarnings(cfg.Tasks) {
		fmt.Printf("%s %s\n", ui.Warning("warning:"), w)
	}

	return nil
}

// validateConfig runs the checks that loading leaves to run time: dependency
// cycles and whether build tasks resolve to a derivation.
func validateConfig(ctx context.Context, cfg *config.Config, builds buildChecker) error {
	var errs []string

	if _, err := runner.NewTaskGraph(cfg.Tasks); err != nil {
		errs = append(errs, err.Error())
	}

	var buildTasks []string
	for _, name := range slices.Sorted(maps.Keys(cfg.Tasks)) {
		if cfg.Tasks[name].Type == config.TaskTypeBuild {
			buildTasks = append(buildTasks, name)
		}
	}
	broken, err := builds.UnbuildableTasks(ctx, nix.CurrentSystem(), buildTasks)
	if err != nil {
		return fmt.Errorf("failed to check build tasks: %w", err)
	}
	for _, name := range broken {
		errs = append(errs, fmt.Sprintf("task '%s': build task has no derivation at %s",
			name, nix.NixTasksBuildsAttr(nix.CurrentSystem(), name)))
	}

	if len(errs) > 0 {
		slices.Sort(errs)
		return &config.ValidationError{Errors: errs}
	}
	return nil
}
