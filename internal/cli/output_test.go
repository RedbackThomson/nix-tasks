package cli

import (
	"os"
	"slices"
	"testing"

	"github.com/posener/complete"
)

func TestResolveOutput(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name     string
		output   string
		verbose  bool
		env      map[string]string
		terminal bool
		want     string
	}{
		{name: "terminal", output: "auto", terminal: true, want: OutputTTY},
		{name: "piped", output: "auto", want: OutputPlain},
		{name: "CI streams", output: "auto", env: map[string]string{"CI": "true"}, terminal: true, want: OutputStream},
		{name: "-v streams", output: "auto", verbose: true, terminal: true, want: OutputStream},
		{name: "explicit mode wins over -v", output: "plain", verbose: true, want: OutputPlain},
		{name: "explicit mode wins over CI", output: "tty", env: map[string]string{"CI": "true"}, want: OutputTTY},
		{name: "GitHub Actions", output: "auto", env: map[string]string{"CI": "true", "GITHUB_ACTIONS": "true"}, want: OutputGitHub},
		{name: "-v wins over GitHub Actions", output: "auto", verbose: true, env: map[string]string{"GITHUB_ACTIONS": "true"}, want: OutputStream},
		{name: "explicit mode wins over GitHub Actions", output: "plain", env: map[string]string{"GITHUB_ACTIONS": "true"}, want: OutputPlain},
		{name: "github anywhere", output: "github", terminal: true, want: OutputGitHub},
		{name: "json only when asked", output: "json", verbose: true, env: map[string]string{"GITHUB_ACTIONS": "true"}, terminal: true, want: OutputJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOutput(tt.output, tt.verbose, env(tt.env), tt.terminal); got != tt.want {
				t.Errorf("resolveOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompletionsOfferOutputModes(t *testing.T) {
	root, parser := completionCommand(t)
	var modes []string
	for _, f := range parser.Model.Flags {
		if f.Name == "output" {
			modes = f.EnumSlice()
		}
	}
	if !slices.Contains(modes, OutputAuto) {
		t.Fatalf("--output enum = %v, want it to include %q", modes, OutputAuto)
	}
	got := root.GlobalFlags["--output"].Predict(complete.Args{})
	slices.Sort(got)
	if want := slices.Sorted(slices.Values(modes)); !slices.Equal(got, want) {
		t.Errorf("--output completes to %v, want %v", got, want)
	}
}

func TestColorEnabled_GitHub(t *testing.T) {
	tests := []struct {
		name    string
		color   string
		output  string
		noColor bool
		want    bool
	}{
		{name: "github mode forces colour on a pipe", color: "auto", output: "github", want: true},
		{name: "NO_COLOR still wins", color: "auto", output: "github", noColor: true, want: false},
		{name: "--color=never still wins", color: "never", output: "github", want: false},
		{name: "plain output on a pipe has no colour", color: "auto", output: "plain", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "1")
			if !tt.noColor {
				_ = os.Unsetenv("NO_COLOR")
			}
			g := &Globals{Color: tt.color, Output: tt.output}
			if got := g.ColorEnabled(); got != tt.want {
				t.Errorf("ColorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
