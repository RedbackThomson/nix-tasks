package cli

import (
	"os"

	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// Output modes accepted by --output.
const (
	OutputAuto   = "auto"
	OutputTTY    = "tty"
	OutputStream = "stream"
	OutputPlain  = "plain"
	OutputGitHub = "github"
	OutputJSON   = "json"
)

// OutputMode resolves --output, -v and the environment to the mode run uses.
func (g *Globals) OutputMode() string {
	return resolveOutput(g.Output, g.Verbose, os.Getenv, ui.IsTerminal(os.Stdout))
}

func resolveOutput(output string, verbose bool, getenv func(string) string, terminal bool) string {
	switch {
	case output != "" && output != OutputAuto:
		return output
	case verbose:
		return OutputStream
	case getenv("GITHUB_ACTIONS") == "true":
		return OutputGitHub
	case getenv("CI") != "":
		return OutputStream
	case terminal:
		return OutputTTY
	}
	return OutputPlain
}

// ColorEnabled resolves --color for the whole process. The GitHub log viewer
// renders colour although stdout is not a terminal, so github output is
// coloured unless NO_COLOR or --color=never says otherwise.
func (g *Globals) ColorEnabled() bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	if ui.ColorMode(g.Color) == ui.ColorAuto && !noColor && g.OutputMode() == OutputGitHub {
		return true
	}
	return ui.ResolveColor(ui.ColorMode(g.Color), os.Stdout)
}
