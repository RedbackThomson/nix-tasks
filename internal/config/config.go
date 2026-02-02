package config

// Config is the root configuration structure
type Config struct {
	Packages  map[string]string `json:"packages"`
	Tasks     map[string]Task   `json:"tasks"`
	DevShells map[string]Shell  `json:"devShells"`
}

// Task represents a single task definition
type Task struct {
	Description string            `json:"description"`
	Deps        []string          `json:"deps"`
	Depends     []string          `json:"depends"`
	Commands    []string          `json:"commands"`
	Script      string            `json:"script"`
	Env         map[string]string `json:"env"`
	WorkingDir  string            `json:"workingDir"`
	Inputs      []string          `json:"inputs"`
	Outputs     []string          `json:"outputs"`

	// Error handling
	ContinueOnError bool `json:"continueOnError"`
}

// Shell represents a dev shell definition
type Shell struct {
	Extends   string            `json:"extends"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
	ShellHook string            `json:"shellHook"`
}
