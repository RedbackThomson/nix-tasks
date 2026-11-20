package cli

import (
	"os"
	"slices"
	"testing"
)

func TestTUIRunsThisExecutableWithForwardedFlags(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		globals Globals
		want    []string
	}{
		{"defaults", Globals{Flake: "/src/app", Color: "auto"}, []string{"-f", "/src/app"}},
		{"verbose and debug", Globals{Flake: "/src/app", Verbose: true, Debug: true}, []string{"-f", "/src/app", "-v", "--debug"}},
		{"colour override", Globals{Flake: "github:org/repo", Color: "never"}, []string{"-f", "github:org/repo", "--color=never"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := tuiOptions(&tt.globals)
			if err != nil {
				t.Fatal(err)
			}
			if opts.Executable != exe {
				t.Errorf("Executable = %q, want %q", opts.Executable, exe)
			}
			if !slices.Equal(opts.Args, tt.want) {
				t.Errorf("Args = %q, want %q", opts.Args, tt.want)
			}
			if opts.FlakePath != tt.globals.Flake {
				t.Errorf("FlakePath = %q, want %q", opts.FlakePath, tt.globals.Flake)
			}
		})
	}
}
