package runner_test

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestFailureStrategy_String(t *testing.T) {
	tests := []struct {
		strategy runner.FailureStrategy
		want     string
	}{
		{runner.FailFast, "fail-fast"},
		{runner.ContinueOnError, "continue-on-error"},
		{runner.FailureStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.strategy.String(); got != tt.want {
				t.Errorf("FailureStrategy.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
