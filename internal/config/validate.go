package config

import (
	"fmt"
	"strings"
)

// ValidationError contains all validation errors found
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed:\n  %s", strings.Join(e.Errors, "\n  "))
}

// Validate checks the configuration for errors
func Validate(cfg *Config) error {
	var errs []string

	// Check task deps reference valid packages
	for taskName, task := range cfg.Tasks {
		for _, dep := range task.Deps {
			if _, ok := cfg.Packages[dep]; !ok {
				errs = append(errs, fmt.Sprintf("task '%s': unknown package '%s'", taskName, dep))
			}
		}

		// Check task depends reference valid tasks
		for _, dep := range task.Depends {
			depName := strings.TrimPrefix(dep, "task:")
			if _, ok := cfg.Tasks[depName]; !ok {
				errs = append(errs, fmt.Sprintf("task '%s': unknown task dependency '%s'", taskName, dep))
			}
		}
	}

	// Check shell packages reference valid packages
	for shellName, shell := range cfg.DevShells {
		for _, pkg := range shell.Packages {
			if _, ok := cfg.Packages[pkg]; !ok {
				errs = append(errs, fmt.Sprintf("shell '%s': unknown package '%s'", shellName, pkg))
			}
		}

		// Check extends references valid shell
		if shell.Extends != "" {
			if _, ok := cfg.DevShells[shell.Extends]; !ok {
				errs = append(errs, fmt.Sprintf("shell '%s': unknown parent shell '%s'", shellName, shell.Extends))
			}
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}
