package cli

// Globals contains flags available to all commands
type Globals struct {
	Verbose bool   `short:"v" aliases:"stream" help:"Stream task output as it runs, prefixed with the task name (default in CI)"`
	Color   string `enum:"auto,always,never" default:"auto" help:"Colour output: auto, always or never. auto colours a terminal unless NO_COLOR is set"`
	Debug   bool   `help:"Show debug information including Nix commands"`
	Flake   string `short:"f" completion-predictor:"dir" help:"Path to flake" default:"."`

	// FlakeRoot is the local directory containing flake.nix, or empty when
	// Flake is not a local path. Set by ResolveFlake.
	FlakeRoot string `kong:"-"`
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
