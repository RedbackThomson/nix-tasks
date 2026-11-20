package config_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &config.Config{
				Packages: []string{"go"},
				Tasks: map[string]config.Task{
					"build": {Deps: []string{"go"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
		{
			name: "unknown package in task",
			config: &config.Config{
				Packages: []string{},
				Tasks: map[string]config.Task{
					"build": {Deps: []string{"go"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: true,
		},
		{
			name: "unknown task dependency",
			config: &config.Config{
				Packages: []string{"go"},
				Tasks: map[string]config.Task{
					"deploy": {Depends: []string{"build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: true,
		},
		{
			name: "valid task dependency",
			config: &config.Config{
				Packages: []string{"go"},
				Tasks: map[string]config.Task{
					"build":  {Deps: []string{"go"}},
					"deploy": {Depends: []string{"build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
		{
			name: "unknown package in shell",
			config: &config.Config{
				Packages: []string{},
				Tasks:    map[string]config.Task{},
				DevShells: map[string]config.Shell{
					"default": {Packages: []string{"go"}},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown parent shell",
			config: &config.Config{
				Packages: []string{"go"},
				Tasks:    map[string]config.Task{},
				DevShells: map[string]config.Shell{
					"default": {Extends: "ci", Packages: []string{"go"}},
				},
			},
			wantErr: true,
		},
		{
			name: "valid shell inheritance",
			config: &config.Config{
				Packages: []string{"go"},
				Tasks:    map[string]config.Task{},
				DevShells: map[string]config.Shell{
					"ci":      {Packages: []string{"go"}},
					"default": {Extends: "ci"},
				},
			},
			wantErr: false,
		},
		{
			name: "unknown after-hook target",
			config: &config.Config{
				Packages: []string{},
				Tasks: map[string]config.Task{
					"hook": {After: []string{"build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: true,
		},
		{
			name: "valid after-hook target",
			config: &config.Config{
				Packages: []string{},
				Tasks: map[string]config.Task{
					"build": {},
					"hook":  {After: []string{"build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
		{
			name: "unknown task type",
			config: &config.Config{
				Tasks: map[string]config.Task{"build": {Type: "docker"}},
			},
			wantErr: true,
		},
		{
			name: "explicit and implicit task types",
			config: &config.Config{
				Tasks: map[string]config.Task{"a": {}, "b": {Type: config.TaskTypeShell}, "c": {Type: config.TaskTypeBuild}},
			},
			wantErr: false,
		},
		{
			name: "empty config",
			config: &config.Config{
				Packages:  []string{},
				Tasks:     map[string]config.Task{},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.wantErr {
				var validErr *config.ValidationError
				if !errors.As(err, &validErr) {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestValidate_ErrorsSorted(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"z": {Deps: []string{"go"}},
			"a": {Depends: []string{"missing"}, Deps: []string{"rust"}},
			"m": {Type: "weird"},
		},
		DevShells: map[string]config.Shell{"ci": {Packages: []string{"jq"}}},
	}
	for i := 0; i < 5; i++ {
		var validErr *config.ValidationError
		if !errors.As(config.Validate(cfg), &validErr) {
			t.Fatal("expected a ValidationError")
		}
		if !slices.IsSorted(validErr.Errors) || len(validErr.Errors) != 5 {
			t.Fatalf("errors not sorted or incomplete: %q", validErr.Errors)
		}
	}
}
