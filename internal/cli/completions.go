package cli

import (
	"fmt"
	"os"
)

// CompletionsCmd generates shell completions
type CompletionsCmd struct {
	Bash BashCompletionsCmd `cmd:"" help:"Generate bash completions"`
	Zsh  ZshCompletionsCmd  `cmd:"" help:"Generate zsh completions"`
	Fish FishCompletionsCmd `cmd:"" help:"Generate fish completions"`
}

// BashCompletionsCmd generates bash completions
type BashCompletionsCmd struct{}

// Run generates bash completions
func (c *BashCompletionsCmd) Run(globals *Globals) error {
	completions := `# nix-tasks bash completion
_nix_tasks_completions() {
    local cur prev words cword
    _init_completion || return

    local commands="run list describe shell cache validate tui completions"
    local global_flags="-v --verbose --debug -f --flake --help"

    case "${prev}" in
        run|describe)
            # Complete task names
            local tasks
            tasks=$(nix-tasks list 2>/dev/null | grep "^  " | awk '{print $1}')
            COMPREPLY=($(compgen -W "${tasks}" -- "${cur}"))
            return 0
            ;;
        shell)
            # Complete shell names
            local shells
            shells=$(nix-tasks list 2>/dev/null | sed -n '/Dev Shells:/,/^$/p' | grep "^  " | awk '{print $1}')
            COMPREPLY=($(compgen -W "${shells} default" -- "${cur}"))
            return 0
            ;;
        -f|--flake)
            # Complete directories
            _filedir -d
            return 0
            ;;
        cache)
            COMPREPLY=($(compgen -W "clean stats" -- "${cur}"))
            return 0
            ;;
        completions)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "${cur}"))
            return 0
            ;;
    esac

    if [[ "${cur}" == -* ]]; then
        COMPREPLY=($(compgen -W "${global_flags}" -- "${cur}"))
        return 0
    fi

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return 0
    fi
}

complete -F _nix_tasks_completions nix-tasks
`
	_, _ = fmt.Fprint(os.Stdout, completions)
	return nil
}

// ZshCompletionsCmd generates zsh completions
type ZshCompletionsCmd struct{}

// Run generates zsh completions
func (c *ZshCompletionsCmd) Run(globals *Globals) error {
	completions := `#compdef nix-tasks

_nix_tasks() {
    local -a commands
    commands=(
        'run:Run a task'
        'list:List available tasks and shells'
        'describe:Show task details'
        'shell:Enter a development shell'
        'cache:Cache management commands'
        'validate:Validate configuration'
        'tui:Launch interactive TUI'
        'completions:Generate shell completions'
    )

    local -a global_flags
    global_flags=(
        '-v[Show task output]'
        '--verbose[Show task output]'
        '--debug[Show debug information]'
        '-f[Path to flake]:flake path:_directories'
        '--flake[Path to flake]:flake path:_directories'
        '--help[Show help]'
    )

    _arguments -C \
        "${global_flags[@]}" \
        '1:command:->command' \
        '*::arg:->args'

    case "$state" in
        command)
            _describe -t commands 'nix-tasks command' commands
            ;;
        args)
            case "${words[1]}" in
                run|describe)
                    local -a tasks
                    tasks=(${(f)"$(nix-tasks list 2>/dev/null | grep "^  " | awk '{print $1}')"})
                    _describe -t tasks 'task' tasks
                    ;;
                shell)
                    local -a shells
                    shells=(${(f)"$(nix-tasks list 2>/dev/null | sed -n '/Dev Shells:/,/^$/p' | grep "^  " | awk '{print $1}')"} 'default')
                    _describe -t shells 'shell' shells
                    ;;
                cache)
                    local -a cache_cmds
                    cache_cmds=('clean:Clear the cache' 'stats:Show cache statistics')
                    _describe -t cache_cmds 'cache command' cache_cmds
                    ;;
                completions)
                    local -a comp_shells
                    comp_shells=('bash' 'zsh' 'fish')
                    _describe -t comp_shells 'shell' comp_shells
                    ;;
            esac
            ;;
    esac
}

_nix_tasks "$@"
`
	_, _ = fmt.Fprint(os.Stdout, completions)
	return nil
}

// FishCompletionsCmd generates fish completions
type FishCompletionsCmd struct{}

// Run generates fish completions
func (c *FishCompletionsCmd) Run(globals *Globals) error {
	completions := `# nix-tasks fish completion

# Disable file completion by default
complete -c nix-tasks -f

# Global flags
complete -c nix-tasks -s v -l verbose -d 'Show task output'
complete -c nix-tasks -l debug -d 'Show debug information'
complete -c nix-tasks -s f -l flake -d 'Path to flake' -r -a '(__fish_complete_directories)'
complete -c nix-tasks -l help -d 'Show help'

# Commands
complete -c nix-tasks -n '__fish_use_subcommand' -a run -d 'Run a task'
complete -c nix-tasks -n '__fish_use_subcommand' -a list -d 'List available tasks and shells'
complete -c nix-tasks -n '__fish_use_subcommand' -a describe -d 'Show task details'
complete -c nix-tasks -n '__fish_use_subcommand' -a shell -d 'Enter a development shell'
complete -c nix-tasks -n '__fish_use_subcommand' -a cache -d 'Cache management commands'
complete -c nix-tasks -n '__fish_use_subcommand' -a validate -d 'Validate configuration'
complete -c nix-tasks -n '__fish_use_subcommand' -a tui -d 'Launch interactive TUI'
complete -c nix-tasks -n '__fish_use_subcommand' -a completions -d 'Generate shell completions'

# Helper function to get tasks
function __nix_tasks_tasks
    nix-tasks list 2>/dev/null | string match -r '^\s+\S+' | string trim
end

# Helper function to get shells
function __nix_tasks_shells
    nix-tasks list 2>/dev/null | sed -n '/Dev Shells:/,/^$/p' | string match -r '^\s+\S+' | string trim
    echo default
end

# Task completions for run and describe
complete -c nix-tasks -n '__fish_seen_subcommand_from run describe' -a '(__nix_tasks_tasks)'

# Shell completions for shell command
complete -c nix-tasks -n '__fish_seen_subcommand_from shell' -a '(__nix_tasks_shells)'

# Cache subcommands
complete -c nix-tasks -n '__fish_seen_subcommand_from cache' -a 'clean' -d 'Clear the cache'
complete -c nix-tasks -n '__fish_seen_subcommand_from cache' -a 'stats' -d 'Show cache statistics'

# Completions subcommands
complete -c nix-tasks -n '__fish_seen_subcommand_from completions' -a 'bash zsh fish'
`
	_, _ = fmt.Fprint(os.Stdout, completions)
	return nil
}
