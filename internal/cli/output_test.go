package cli

import (
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
