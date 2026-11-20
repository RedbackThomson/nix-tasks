package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// fakeEvaluator serves canned JSON per attribute; attributes listed in
// missing fail the way nix does for an absent flake output.
type fakeEvaluator struct {
	values  map[string]string
	missing map[string]bool
	errs    map[string]error
	calls   []string
}

func (f *fakeEvaluator) Eval(_ context.Context, attr string, result any) error {
	f.calls = append(f.calls, attr)
	if f.missing[attr] {
		return &nix.EvalError{
			Attribute: attr,
			Stderr:    "error: flake 'path:/x' does not provide attribute 'packages.x.y', 'legacyPackages.x.y' or '" + attr + "'",
			Err:       errors.New("exit status 1"),
		}
	}
	if err, ok := f.errs[attr]; ok {
		return err
	}
	v, ok := f.values[attr]
	if !ok {
		return errors.New("unexpected attribute " + attr)
	}
	return json.Unmarshal([]byte(v), result)
}

func systemAttr() string {
	return "nixTasksConfig." + nix.QuoteAttr(nix.CurrentSystem())
}

func TestLoad(t *testing.T) {
	sys := systemAttr()

	tests := []struct {
		name      string
		eval      *fakeEvaluator
		wantErr   string
		wantCalls int
		check     func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "per-system config",
			eval: &fakeEvaluator{values: map[string]string{
				sys: `{"packages":{},"tasks":{"hello":{"type":"shell","commands":["echo hi"],"env":{"PORT":"8080"}}},"devShells":{}}`,
			}},
			wantCalls: 1,
			check: func(t *testing.T, cfg *config.Config) {
				if got := cfg.Tasks["hello"].Env["PORT"]; got != "8080" {
					t.Errorf("env PORT = %q, want 8080", got)
				}
			},
		},
		{
			name: "falls back when per-system attribute is missing",
			eval: &fakeEvaluator{
				missing: map[string]bool{sys: true},
				values:  map[string]string{"nixTasksConfig": `{"tasks":{"hello":{"commands":["echo hi"]}}}`},
			},
			wantCalls: 2,
			check: func(t *testing.T, cfg *config.Config) {
				if _, ok := cfg.Tasks["hello"]; !ok {
					t.Errorf("expected task from fallback config, got %v", cfg.Tasks)
				}
			},
		},
		{
			name: "evaluation error does not fall back",
			eval: &fakeEvaluator{
				errs:   map[string]error{sys: &nix.EvalError{Attribute: sys, Stderr: "error: attribute 'go' missing", Err: errors.New("exit status 1")}},
				values: map[string]string{"nixTasksConfig": `{"tasks":{}}`},
			},
			wantErr:   "attribute 'go' missing",
			wantCalls: 1,
		},
		{
			name: "decode error names the task and does not fall back",
			eval: &fakeEvaluator{values: map[string]string{
				sys:              `{"tasks":{"web":{"env":{"PORT":8080}}}}`,
				"nixTasksConfig": `{"tasks":{}}`,
			}},
			wantErr:   "tasks.web",
			wantCalls: 1,
		},
		{
			name: "per-system attrset decoded as config is rejected",
			eval: &fakeEvaluator{
				missing: map[string]bool{sys: true},
				values: map[string]string{
					"nixTasksConfig": `{"aarch64-darwin":{"tasks":{}},"x86_64-linux":{"tasks":{}}}`,
				},
			},
			wantErr:   "none of packages, tasks, devShells found",
			wantCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(context.Background(), tt.eval)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(tt.eval.calls) != tt.wantCalls {
				t.Errorf("evaluated %v, want %d evaluations", tt.eval.calls, tt.wantCalls)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}
