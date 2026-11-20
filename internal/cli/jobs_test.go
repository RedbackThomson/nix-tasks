package cli_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/redbackthomson/nix-tasks/internal/cli"
)

func TestJobsPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     string
		args    []string
		want    int
		wantErr string
	}{
		{"default is the CPUs available", "", nil, runtime.GOMAXPROCS(0), ""},
		{"environment overrides the default", "3", nil, 3, ""},
		{"flag overrides the environment", "3", []string{"-j", "5"}, 5, ""},
		{"flag overrides an invalid environment", "lots", []string{"-j", "5"}, 5, ""},
		{"environment must be a number", "lots", nil, 0, `NIX_TASKS_JOBS must be a whole number, got "lots"`},
		{"environment must be positive", "0", nil, 0, "NIX_TASKS_JOBS must be at least 1, got 0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NIX_TASKS_JOBS", tt.env)
			var c cli.CLI
			parser, err := kong.New(&c)
			if err != nil {
				t.Fatal(err)
			}
			_, err = parser.Parse(append([]string{"run", "build"}, tt.args...))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *c.Run.Jobs != tt.want {
				t.Errorf("jobs = %d, want %d", *c.Run.Jobs, tt.want)
			}
		})
	}
}
