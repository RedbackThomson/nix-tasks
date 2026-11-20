//go:build integration

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// Completing `run` must offer the flake's tasks (not its dev shells), read
// from the flake given with -f rather than the working directory.
func TestCompletions_BashUsesListNames(t *testing.T) {
	skipIfNoNix(t)
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  tasks.build.commands = [ "true" ];
  tasks.test.commands = [ "true" ];
  devShells.extra = { };
}`, map[string]string{})

	complete := func(words ...string) []string {
		t.Helper()
		script := `source <(nix-tasks completions bash)
COMP_LINE="$1" COMP_POINT=${#1}
_nix_tasks
printf '%s\n' "${COMPREPLY[@]}"`
		cmd := exec.Command("bash", "-c", script, "bash", strings.Join(words, " "))
		cmd.Dir = t.TempDir()
		cmd.Env = append(testEnviron(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("completion failed: %v\n%s", err, out)
		}
		got := strings.Fields(string(out))
		slices.Sort(got)
		return got
	}

	if got := complete("nix-tasks", "-f", dir, "run", ""); !slices.Equal(got, []string{"build", "test"}) {
		t.Errorf("run completions = %q, want [build test]", got)
	}
	if got := complete("nix-tasks", "-f", dir, "shell", ""); !slices.Equal(got, []string{"extra"}) {
		t.Errorf("shell completions = %q, want [extra]", got)
	}
	if got := complete("nix-tasks", "gr"); !slices.Equal(got, []string{"graph"}) {
		t.Errorf("command completions = %q, want [graph]", got)
	}
}
