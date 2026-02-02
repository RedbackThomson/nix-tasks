package cli

// Globals contains flags available to all commands
type Globals struct {
	Verbose bool   `short:"v" help:"Show task output"`
	Debug   bool   `help:"Show debug information including Nix commands"`
	Flake   string `short:"f" help:"Path to flake" default:"."`
}

// CLI is the root command structure
type CLI struct {
	Globals

	Run      RunCmd      `cmd:"" help:"Run a task"`
	List     ListCmd     `cmd:"" help:"List available tasks"`
	Describe DescribeCmd `cmd:"" help:"Show task details"`
	Validate ValidateCmd `cmd:"" help:"Validate configuration"`
}
