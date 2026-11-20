package cli

// Globals contains flags available to all commands
type Globals struct {
	Verbose bool   `short:"v" aliases:"stream" help:"Same as --output=stream"`
	Output  string `enum:"auto,tty,stream,plain,github" default:"auto" help:"How run shows tasks: tty (live display), stream (every output line, prefixed), plain (start and result lines), github (GitHub Actions log, summary and annotations) or auto: github under GitHub Actions, stream in other CI, tty on a terminal, else plain"`
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

	Run         RunCmd         `cmd:"" help:"Run one or more tasks"`
	List        ListCmd        `cmd:"" help:"List available tasks and shells"`
	Describe    DescribeCmd    `cmd:"" help:"Show task details"`
	Graph       GraphCmd       `cmd:"" help:"Show execution graph for a task"`
	Shell       ShellCmd       `cmd:"" help:"Enter a development shell"`
	Cache       CacheCmd       `cmd:"" help:"Cache management commands"`
	Validate    ValidateCmd    `cmd:"" help:"Validate configuration"`
	TUI         TUICmd         `cmd:"" default:"withargs" help:"Launch interactive TUI (default; prints the list when stdout is not a terminal)"`
	Completions CompletionsCmd `cmd:"" help:"Generate shell completions"`
}
