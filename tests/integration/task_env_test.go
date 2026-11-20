//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func nixSystem() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	return arch + "-" + runtime.GOOS
}

func TestTaskEnv_GoBuildUsesShellPackages(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  packages.go = pkgs.go;
  tasks.build = {
    deps = [ "go" ];
    commands = [ "go build -o app ." "./app > out.txt" ];
  };
}`, map[string]string{
		"go.mod":  "module example.com/app\n\ngo 1.21\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"built by go\") }\n",
	})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "run", "build")
	if code != 0 {
		t.Fatalf("run build exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	out, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "built by go" {
		t.Errorf("out.txt = %q", out)
	}
}

func TestTaskEnv_PathKeepsShellTools(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  packages.hello = pkgs.hello;
  tasks.greet = {
    deps = [ "hello" ];
    env.PATH = "/task/bin";
    commands = [ "hello > out.txt" "echo $PATH > path.txt" ];
  };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "run", "greet")
	if code != 0 {
		t.Fatalf("run greet exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	out, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Hello, world!") {
		t.Errorf("out.txt = %q", out)
	}
	path, err := os.ReadFile(filepath.Join(dir, "path.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(path), "/task/bin:/nix/store/") {
		t.Errorf("PATH should start with the task's entry, then the shell's: %q", path)
	}
}

// parseEnv0 parses `env -0` output.
func parseEnv0(t *testing.T, data []byte) map[string]string {
	t.Helper()
	vars := map[string]string{}
	for _, kv := range bytes.Split(bytes.TrimRight(data, "\x00"), []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if !ok {
			t.Fatalf("malformed env entry %q", kv)
		}
		vars[k] = v
	}
	return vars
}

func TestTaskEnv_MatchesNixDevelop(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  packages.go = pkgs.go;
  tasks.env = {
    deps = [ "go" ];
    env.PATH = "/task/bin";
    commands = [ "env -0 > task-env" ];
  };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "run", "env")
	if code != 0 {
		t.Fatalf("run env exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	taskData, err := os.ReadFile(filepath.Join(dir, "task-env"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	drv, err := exec.CommandContext(ctx, "nix", "eval", "--raw", dir+"#nixTasksShellDrvs."+nixSystem()+".env").Output()
	if err != nil {
		t.Fatalf("nix eval drvPath: %v", err)
	}
	develop := exec.CommandContext(ctx, "nix", "develop", string(drv)+"^*", "--command", "env", "-0")
	develop.Dir = dir
	develop.Env = testEnviron()
	developData, err := develop.Output()
	if err != nil {
		t.Fatalf("nix develop: %v", err)
	}

	task := parseEnv0(t, taskData)
	want := parseEnv0(t, developData)
	// A task's PATH goes in front of the shell's.
	want["PATH"] = "/task/bin:" + want["PATH"]

	// nix develop gives each shell a fresh temp directory and a GC root
	// variable; nix-tasks keeps the host's temp directory instead. The rest
	// are per-process bash bookkeeping or set by the test harness.
	differ := map[string]bool{
		"NIX_BUILD_TOP": true, "TMP": true, "TMPDIR": true, "TEMP": true, "TEMPDIR": true,
		"NIX_GCROOT": true, "SHLVL": true, "_": true, "PWD": true, "OLDPWD": true,
		"NIX_TASKS_CACHE_DIR": true, "NIX_TASKS_TEST_LOG_DIR": true,
	}
	trim := func(k, v string) string {
		if k == "XDG_DATA_DIRS" {
			return strings.Trim(v, ":")
		}
		return v
	}
	for k, v := range want {
		if differ[k] {
			continue
		}
		got, ok := task[k]
		if !ok {
			t.Errorf("%s missing from the task environment (nix develop: %q)", k, v)
		} else if trim(k, got) != trim(k, v) {
			t.Errorf("%s = %q, nix develop has %q", k, got, v)
		}
	}
	for k, v := range task {
		if _, ok := want[k]; !ok && !differ[k] {
			t.Errorf("%s = %q is not set by nix develop", k, v)
		}
	}
}

func TestTaskEnv_ShellsRealisedOncePerRun(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)

	dir := writeTempFlake(t, `{
  packages = { go = pkgs.go; hello = pkgs.hello; };
  tasks = {
    a = { deps = [ "hello" ]; commands = [ "hello > a.txt" ]; };
    b = { deps = [ "hello" ]; commands = [ "hello > b.txt" ]; };
    c = { deps = [ "go" ]; commands = [ "go version > c.txt" ]; };
    all = tasksLib.mkCompoundTask { tasks = [ "a" "b" "c" ]; };
  };
}`, map[string]string{})
	cacheDir := t.TempDir()

	_, stderr, err := runNixTasksInDir(t, binary, dir, cacheDir, "--debug", "run", "all")
	if err != nil {
		t.Fatalf("first run failed: %v\nstderr: %s", err, stderr)
	}
	if n := strings.Count(stderr, `msg="evaluated task shells"`); n != 1 {
		t.Errorf("task shells evaluated %d times, want 1", n)
	}
	// a and b share a shell, and the compound task needs none.
	if n := strings.Count(stderr, `msg="realised task shell"`); n != 2 {
		t.Errorf("realised %d shells, want 2\nstderr: %s", n, stderr)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}

	_, stderr, err = runNixTasksInDir(t, binary, dir, cacheDir, "--debug", "run", "all")
	if err != nil {
		t.Fatalf("second run failed: %v\nstderr: %s", err, stderr)
	}
	if n := strings.Count(stderr, `msg="realised task shell"`); n != 0 {
		t.Errorf("second run realised %d shells, want 0 (cached)", n)
	}
	if n := strings.Count(stderr, `msg="dev env cache hit"`); n != 2 {
		t.Errorf("second run had %d dev env cache hits, want 2", n)
	}
}
