package config

import (
	"fmt"
	"strings"
)

// nameErrors rejects task and shell names that contain a double quote: nix-tasks
// selects tasks and shells by quoted flake attribute paths, and those quotes
// cannot contain one.
func nameErrors(cfg *Config) []string {
	var errs []string
	for name := range cfg.Tasks {
		if strings.Contains(name, `"`) {
			errs = append(errs, fmt.Sprintf("task '%s': names cannot contain '\"' because nix cannot select them in a flake attribute path; rename the task", name))
		}
	}
	for name := range cfg.DevShells {
		if strings.Contains(name, `"`) {
			errs = append(errs, fmt.Sprintf("shell '%s': names cannot contain '\"' because nix cannot select them in a flake attribute path; rename the shell", name))
		}
	}
	return errs
}
