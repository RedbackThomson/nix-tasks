//go:build integration

package integration_test

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeTempFlake creates a flake outside the repository so tests can add
// files (such as unique build inputs) without them being tracked by git.
// tasksConfig is the attrset passed to lib.evalConfig, with pkgs and lib in
// scope.
func writeTempFlake(t *testing.T, tasksConfig string, files map[string]string) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join(testdataDir(t), "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	flake := fmt.Sprintf(`{
  inputs.nix-tasks.url = "path:%s";

  outputs = { self, nix-tasks }:
    let
      nixpkgs = nix-tasks.inputs.nixpkgs;
      forAllSystems = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      mkConfig = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          lib = nix-tasks.lib.${system};
        in lib.evalConfig (%s);
    in {
      nixTasksConfig = forAllSystems (system: (mkConfig system).nixTasksConfig);
      nixTasksShells = forAllSystems (system: (mkConfig system).nixTasksShells);
      nixTasksBuilds = forAllSystems (system: (mkConfig system).nixTasksBuilds);
      devShells = forAllSystems (system: (mkConfig system).devShells);
    };
}
`, root, tasksConfig)

	files["flake.nix"] = flake
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runNixTasksIn runs nix-tasks with dir as the working directory and returns
// stdout, stderr and the exit code (-1 if it did not exit normally).
func runNixTasksIn(t *testing.T, binary, dir string, args ...string) (string, string, int) {
	t.Helper()

	stdout, stderr, err := runNixTasksInDir(t, binary, dir, t.TempDir(), args...)
	code := 0
	if err != nil {
		code = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
	}
	return stdout, stderr, code
}

func randomString(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func namesAndExitCodesFlake(t *testing.T) string {
	t.Helper()
	return filepath.Join(testdataDir(t), "names-and-exit-codes")
}
