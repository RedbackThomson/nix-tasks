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
	// TaskTypeBuild builds a Nix derivation
	TaskTypeBuild TaskType = "build"
)

// Task represents a single task definition
type Task struct {
	Type        TaskType `json:"type"`
	Description string   `json:"description"`
	Deps        []string `json:"deps"`
	Depends     []string `json:"depends"`

	// Shell task fields
	Commands   []string          `json:"commands,omitempty"`
	Script     string            `json:"script,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"workingDir,omitempty"`

	// Build task fields
	DrvPath        string            `json:"drvPath,omitempty"`        // Derivation path for nix build
	Outputs        map[string]string `json:"outputs,omitempty"`        // Output name -> store path
	DerivationName string            `json:"derivationName,omitempty"` // Derivation name

	// Unused fields (keep for future)
	Inputs          []string `json:"inputs,omitempty"`
	ContinueOnError bool     `json:"continueOnError,omitempty"`
}

// Shell represents a dev shell definition
type Shell struct {
	Extends   string            `json:"extends"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
	ShellHook string            `json:"shellHook"`
}
