//go:build integration

package integration_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestList_Names(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flake := filepath.Join(testdataDir(t), "shells")

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"list", "--names"}, "check-curl\ncheck-jq\n"},
		{[]string{"list", "--names", "--shells"}, "default\nextended\nminimal\n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stdout, stderr, err := runNixTasks(t, binary, append([]string{"-f", flake}, tt.args...)...)
			if err != nil {
				t.Fatalf("%v\nstderr: %s", err, stderr)
			}
			if stdout != tt.want {
				t.Errorf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}
