package cli

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

type fakeBuilds struct {
	broken []string
	asked  []string
	err    error
}

func (f *fakeBuilds) UnbuildableTasks(_ context.Context, _ string, tasks []string) ([]string, error) {
	f.asked = tasks
	return f.broken, f.err
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name   string
		tasks  map[string]config.Task
		builds *fakeBuilds
		want   []string
	}{
		{
			name: "valid",
			tasks: map[string]config.Task{
				"build": {Type: config.TaskTypeBuild},
				"test":  {Depends: []string{"build"}},
			},
			builds: &fakeBuilds{},
		},
		{
			name: "dependency cycle",
			tasks: map[string]config.Task{
				"a": {Depends: []string{"b"}},
				"b": {Depends: []string{"a"}},
			},
			builds: &fakeBuilds{},
			want:   []string{"circular dependency detected: a -> b -> a"},
		},
		{
			name:   "self dependency",
			tasks:  map[string]config.Task{"a": {After: []string{"a"}}},
			builds: &fakeBuilds{},
			want:   []string{"circular dependency detected: a -> a"},
		},
		{
			name: "every problem is listed in order",
			tasks: map[string]config.Task{
				"img": {Type: config.TaskTypeBuild},
				"bin": {Type: config.TaskTypeBuild},
				"x":   {Depends: []string{"x"}},
			},
			builds: &fakeBuilds{broken: []string{"bin", "img"}},
			want: []string{
				"circular dependency detected: x -> x",
				"task 'bin': build task has no derivation",
				"task 'img': build task has no derivation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(context.Background(), &config.Config{Tasks: tt.tasks}, tt.builds)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validErr *config.ValidationError
			if !errors.As(err, &validErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
			if len(validErr.Errors) != len(tt.want) {
				t.Fatalf("errors = %q, want %d", validErr.Errors, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.HasPrefix(validErr.Errors[i], want) {
					t.Errorf("error %d = %q, want prefix %q", i, validErr.Errors[i], want)
				}
			}
		})
	}
}

func TestValidateConfig_OnlyChecksBuildTasks(t *testing.T) {
	builds := &fakeBuilds{}
	cfg := &config.Config{Tasks: map[string]config.Task{
		"z":    {Type: config.TaskTypeBuild},
		"a":    {Type: config.TaskTypeBuild},
		"test": {Type: config.TaskTypeShell},
	}}
	if err := validateConfig(context.Background(), cfg, builds); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(builds.asked, []string{"a", "z"}) {
		t.Errorf("checked %v, want [a z]", builds.asked)
	}
}
