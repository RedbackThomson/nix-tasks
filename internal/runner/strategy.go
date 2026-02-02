package runner

// FailureStrategy defines how to handle task failures
type FailureStrategy int

const (
	// FailFast stops execution on first failure
	FailFast FailureStrategy = iota
	// ContinueOnError continues with independent tasks
	ContinueOnError
)

// String returns the string representation of the strategy
func (s FailureStrategy) String() string {
	switch s {
	case FailFast:
		return "fail-fast"
	case ContinueOnError:
		return "continue-on-error"
	default:
		return "unknown"
	}
}
