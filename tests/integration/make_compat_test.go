//go:build integration

package integration_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func makeCompatFlake(t *testing.T) string {
	t.Helper()
	return filepath.Join(testdataDir(t), "make-compat")
}

func TestMakeCompat(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	tests := []struct {
		name string
		task string
		want []string
	}{
		{name: "make from stdenv", task: "greet", want: []string{"make greet ran"}},
		{name: "flag with a space is one argument", task: "spaced-flag", want: []string{"greeting=hello world"}},
		{name: "makefile path with a space", task: "spaced-makefile", want: []string{"custom makefile ran"}},
		{name: "imported targets keep depends and workingDir", task: "sub-where", want: []string{"prep ran", "make ran in sub"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := runNixTasks(t, binary, "run", tt.task, "--stream", "-f", makeCompatFlake(t))
			if err != nil {
				t.Fatalf("run %s failed: %v\nstdout: %s\nstderr: %s", tt.task, err, stdout, stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("expected %q in output, got: %s", want, stdout)
				}
			}
		})
	}
}
