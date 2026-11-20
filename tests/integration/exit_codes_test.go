//go:build integration

package integration_test

import (
	"testing"
)

func TestShell_CommandExitCode(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := namesAndExitCodesFlake(t)

	_, stderr, code := runNixTasksIn(t, binary, dir, "shell", "-c", "exit 3", "-f", dir)
	if code != 3 {
		t.Errorf("shell -c 'exit 3' exited %d, want 3\nstderr: %s", code, stderr)
	}
}

func TestRun_ExitCodePropagation(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := namesAndExitCodesFlake(t)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"failing task", []string{"run", "exit-four"}, 4},
		{"failing task in raw mode", []string{"run", "--raw", "exit-four"}, 4},
		{"failing dependency", []string{"run", "after-exit-four"}, 4},
		{"multiple failures", []string{"run", "both-fail"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runNixTasksIn(t, binary, dir, append(tt.args, "-f", dir)...)
			if code != tt.want {
				t.Errorf("exited %d, want %d\nstdout: %s\nstderr: %s", code, tt.want, stdout, stderr)
			}
		})
	}
}
