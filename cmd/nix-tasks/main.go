package main

import (
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
	err := ctx.Run(&rootCmd.Globals)
	if err != nil {
		os.Exit(1)
	}
}
