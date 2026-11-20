//go:build integration

package integration_test

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runInPTY runs binary under script(1) so it sees a terminal on stdout and
// returns everything it wrote to that terminal.
func runInPTY(t *testing.T, binary string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "script", append([]string{"-q", "/dev/null", binary}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, "script", "-qec", shellQuote(append([]string{binary}, args...)), "/dev/null")
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%v timed out; output: %q", args, out)
	}
	if err != nil {
		t.Fatalf("script %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func shellQuote(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// Commands other than the TUI must not query the terminal: the replies leak
// into the shell prompt when nix-tasks exits before reading them.
func TestTUI_NoTerminalQueriesOutsideTUI(t *testing.T) {
	out := runInPTY(t, nixTasksBinary(t), "--help")

	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected help output, got %q", out)
	}
	for _, query := range []string{"\x1b]11;?", "\x1b[6n"} {
		if strings.Contains(out, query) {
			t.Errorf("expected no terminal query %q, got %q", query, out)
		}
	}
}
