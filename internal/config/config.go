package config

// TaskDependencyPrefix is the prefix used for task dependencies
const TaskDependencyPrefix = "task:"

// Config is the root configuration structure
type Config struct {
	Packages  map[string]string `json:"packages"`
	Tasks     map[string]Task   `json:"tasks"`
	DevShells map[string]Shell  `json:"devShells"`
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
	Deps        []string `json:"deps"`
	Depends     []string `json:"depends"`
	After       []string `json:"after,omitempty"`

	// Shell task fields
	Commands   []string          `json:"commands,omitempty"`
	Script     string            `json:"script,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"workingDir,omitempty"`

	// Cache control
	NoCache bool `json:"noCache,omitempty"`

	// Input tracking for cache fingerprinting
	Inputs []string `json:"inputs,omitempty"`

	// Error handling
	ContinueOnError bool `json:"continueOnError,omitempty"`
}

// Shell represents a dev shell definition
type Shell struct {
	Extends   string            `json:"extends"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
	ShellHook string            `json:"shellHook"`
}
