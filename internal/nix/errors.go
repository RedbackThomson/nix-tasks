package nix

import (
	"errors"
	"fmt"
	"strings"
)

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

// IsMissingAttribute reports whether err is an evaluation failure caused by
// the flake not providing the requested attribute at all, as opposed to an
// error while evaluating it.
func IsMissingAttribute(err error) bool {
	var evalErr *EvalError
	return errors.As(err, &evalErr) && strings.Contains(evalErr.Stderr, "does not provide attribute")
}
