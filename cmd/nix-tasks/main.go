package main

import (
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/redbackthomson/nix-tasks/internal/cli"
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

	err := ctx.Run(&rootCmd.Globals)
	ctx.FatalIfErrorf(err)
}
