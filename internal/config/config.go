package config

// TaskDependencyPrefix is the prefix used for task dependencies
const TaskDependencyPrefix = "task:"

// Config is the root configuration structure
type Config struct {
	// Packages holds package names only: resolving store paths would force
	// every package to be evaluated whenever the config is loaded.
	Packages      []string         `json:"packages"`
	Tasks         map[string]Task  `json:"tasks"`
	DevShells     map[string]Shell `json:"devShells"`
	CacheSettings CacheSettings    `json:"cacheSettings"`
}

// CacheSettings holds flake-wide cache settings.
type CacheSettings struct {
	// MaxEntriesPerTask is zero when unset.
	MaxEntriesPerTask int `json:"maxEntriesPerTask,omitempty"`
}

// TaskType represents the execution type of a task
type TaskType string

const (
	// TaskTypeShell executes shell commands in nix develop
	TaskTypeShell TaskType = "shell"
	// TaskTypeBuild builds the task's nixTasksBuilds flake attribute
	TaskTypeBuild TaskType = "build"
)

// Task represents a single task definition
type Task struct {
	Type        TaskType `json:"type"`
	Description string   `json:"description"`
	// Label is a short name for the task in streamed output; see Labels.
	Label   string   `json:"label,omitempty"`
	Deps    []string `json:"deps"`
	Depends []string `json:"depends"`
	After   []string `json:"after,omitempty"`

	// Shell task fields
	Commands   []string          `json:"commands,omitempty"`
	Script     string            `json:"script,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"workingDir,omitempty"`

	// Cache enables the task cache; nil means the task always runs.
	Cache *TaskCache `json:"cache,omitempty"`

	// ShellDrvPath identifies the exact toolchain of the task's shell. Only
	// set for cached tasks, so loading the config stays cheap.
	ShellDrvPath string `json:"shellDrvPath,omitempty"`

	// Error handling
	ContinueOnError bool `json:"continueOnError,omitempty"`
}

// TaskCache declares what a cached task reads and writes.
type TaskCache struct {
	// Inputs and Outputs are globs relative to the flake root; a leading
	// "!" excludes.
	Inputs []string `json:"inputs"`
	// Outputs is nil when the configuration omitted it, which is an error:
	// an empty list must be written out.
	Outputs []string `json:"outputs"`
	Env     []string `json:"env,omitempty"`
	// MaxEntries is zero when unset.
	MaxEntries int `json:"maxEntries,omitempty"`
}

// Shell represents a dev shell definition
type Shell struct {
	Extends   string            `json:"extends"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
	ShellHook string            `json:"shellHook"`
}
