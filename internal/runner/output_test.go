package runner_test

import (
	"os"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestDetectOutputMode(t *testing.T) {
	// Save and restore CI env var
	origCI := os.Getenv("CI")
	defer func() {
		if origCI != "" {
			os.Setenv("CI", origCI)
		} else {
			os.Unsetenv("CI")
		}
	}()

	// Test without CI
	os.Unsetenv("CI")
	if got := runner.DetectOutputMode(); got != runner.Buffered {
		t.Errorf("DetectOutputMode() without CI = %v, want Buffered", got)
	}

	// Test with CI
	os.Setenv("CI", "true")
	if got := runner.DetectOutputMode(); got != runner.Streaming {
		t.Errorf("DetectOutputMode() with CI = %v, want Streaming", got)
	}
}
