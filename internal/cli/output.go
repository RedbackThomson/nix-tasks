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
)

// OutputMode resolves --output, -v and the environment to the mode run uses.
func (g *Globals) OutputMode() string {
	return resolveOutput(g.Output, g.Verbose, os.Getenv, ui.IsTerminal(os.Stdout))
}

func resolveOutput(output string, verbose bool, getenv func(string) string, terminal bool) string {
	switch {
	case output != "" && output != OutputAuto:
		return output
	case verbose || getenv("CI") != "":
		return OutputStream
	case terminal:
		return OutputTTY
	}
	return OutputPlain
}

// ColorEnabled resolves --color for the whole process.
func (g *Globals) ColorEnabled() bool {
	return ui.ResolveColor(ui.ColorMode(g.Color), os.Stdout)
}
