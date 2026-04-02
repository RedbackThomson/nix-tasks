package config_test

import (
	"errors"
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
				Packages: map[string]string{"go": "/nix/store/abc-go"},
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
				Packages: map[string]string{},
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
				Packages: map[string]string{"go": "/nix/store/abc-go"},
				Tasks: map[string]config.Task{
					"deploy": {Depends: []string{"task:build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: true,
		},
		{
			name: "valid task dependency",
			config: &config.Config{
				Packages: map[string]string{"go": "/nix/store/abc-go"},
				Tasks: map[string]config.Task{
					"build":  {Deps: []string{"go"}},
					"deploy": {Depends: []string{"task:build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
		{
			name: "unknown package in shell",
			config: &config.Config{
				Packages: map[string]string{},
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
				Packages: map[string]string{"go": "/nix/store/abc-go"},
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
				Packages: map[string]string{"go": "/nix/store/abc-go"},
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
				Packages: map[string]string{},
				Tasks: map[string]config.Task{
					"hook": {After: []string{"task:build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: true,
		},
		{
			name: "valid after-hook target",
			config: &config.Config{
				Packages: map[string]string{},
				Tasks: map[string]config.Task{
					"build": {},
					"hook":  {After: []string{"task:build"}},
				},
				DevShells: map[string]config.Shell{},
			},
			wantErr: false,
		},
		{
			name: "empty config",
			config: &config.Config{
				Packages:  map[string]string{},
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
