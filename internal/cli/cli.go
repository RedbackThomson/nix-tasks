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

	Run         RunCmd         `cmd:"" help:"Run a task"`
	List        ListCmd        `cmd:"" help:"List available tasks and shells"`
	Describe    DescribeCmd    `cmd:"" help:"Show task details"`
	Graph       GraphCmd       `cmd:"" help:"Show execution graph for a task"`
	Shell       ShellCmd       `cmd:"" help:"Enter a development shell"`
	Cache       CacheCmd       `cmd:"" help:"Cache management commands"`
	Validate    ValidateCmd    `cmd:"" help:"Validate configuration"`
	TUI         TUICmd         `cmd:"" default:"withargs" help:"Launch interactive TUI (default)"`
	Completions CompletionsCmd `cmd:"" help:"Generate shell completions"`
}
