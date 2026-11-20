package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/redbackthomson/nix-tasks/internal/cli"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

func main() {
	var rootCmd cli.CLI
	ctx := kong.Parse(&rootCmd,
		kong.Name("nix-tasks"),
		kong.Description("Nix-based task runner"),
		kong.UsageOnError(),
	)

	if rootCmd.Globals.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	ui.SetColor(ui.ResolveColor(ui.ColorMode(rootCmd.Globals.Color), os.Stdout))
	ctx.FatalIfErrorf(rootCmd.Globals.ResolveFlake())

	err := ctx.Run(&rootCmd.Globals)

	var exitErr *cli.ExitCodeError
	if errors.As(err, &exitErr) {
		ctx.Errorf("%s", err)
		ctx.Exit(exitErr.Code)
	}
	ctx.FatalIfErrorf(err)
}
