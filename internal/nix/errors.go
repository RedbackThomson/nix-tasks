package nix

import "fmt"

// EvalError represents a Nix evaluation error
type EvalError struct {
	Attribute string
	Stderr    string
	Err       error
}

func (e *EvalError) Error() string {
	return fmt.Sprintf("nix eval '%s' failed: %v\n%s", e.Attribute, e.Err, e.Stderr)
}

func (e *EvalError) Unwrap() error {
	return e.Err
}

// BuildError represents a Nix build error
type BuildError struct {
	Attribute string
	Stderr    string
	Err       error
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("nix build '%s' failed: %v\n%s", e.Attribute, e.Err, e.Stderr)
}

func (e *BuildError) Unwrap() error {
	return e.Err
}
