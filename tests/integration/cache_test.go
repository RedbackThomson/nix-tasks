//go:build integration

package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// cacheFlake is a temporary copy of testdata/cache with one input file, and
// a cache directory shared by every run in the test.
type cacheFlake struct {
	t        *testing.T
	binary   string
	dir      string
	cacheDir string
}

func newCacheFlake(t *testing.T) *cacheFlake {
	t.Helper()
	skipIfNoNix(t)
	module, err := os.ReadFile(filepath.Join(testdataDir(t), "cache", "tasks.nix"))
	if err != nil {
		t.Fatal(err)
	}
	return &cacheFlake{
		t:        t,
		binary:   nixTasksBinary(t),
		dir:      writeTempFlake(t, string(module), map[string]string{"src/a.txt": "A\n", "tool.txt": "v1\n"}),
		cacheDir: t.TempDir(),
	}
}

func (f *cacheFlake) run(args ...string) (stdout, stderr string, err error) {
	f.t.Helper()
	return runNixTasksInDir(f.t, f.binary, f.dir, f.cacheDir, append(args, "-f", f.dir)...)
}

// mustRun runs a task and fails the test unless it succeeds.
func (f *cacheFlake) mustRun(args ...string) string {
	f.t.Helper()
	stdout, stderr, err := f.run(append([]string{"run"}, args...)...)
	if err != nil {
		f.t.Fatalf("run %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stripANSI(stdout)
}

func (f *cacheFlake) write(rel, content string) {
	f.t.Helper()
	path := filepath.Join(f.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *cacheFlake) read(rel string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, rel))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

// runs returns how many times each task has really executed.
func (f *cacheFlake) runs() map[string]int {
	f.t.Helper()
	counts := map[string]int{}
	data, err := os.ReadFile(filepath.Join(f.dir, "runs.log"))
	if os.IsNotExist(err) {
		return counts
	}
	if err != nil {
		f.t.Fatal(err)
	}
	for _, name := range strings.Fields(string(data)) {
		counts[name]++
	}
	return counts
}

func (f *cacheFlake) assertRuns(task string, want int) {
	f.t.Helper()
	if got := f.runs()[task]; got != want {
		f.t.Errorf("%s executed %d times, want %d", task, got, want)
	}
}

// hit reports whether out shows task answered from the cache.
func hit(out, task string) bool {
	return strings.Contains(out, "✓ "+task+" (cached")
}

func TestCache_SecondRunHits(t *testing.T) {
	f := newCacheFlake(t)
	if out := f.mustRun("generate"); strings.Contains(out, "cached") {
		t.Errorf("first run should execute:\n%s", out)
	}
	out := f.mustRun("generate")
	if !hit(out, "generate") {
		t.Errorf("second run should hit:\n%s", out)
	}
	f.assertRuns("generate", 1)
}

func TestCache_InputEditReruns(t *testing.T) {
	f := newCacheFlake(t)
	f.mustRun("generate")
	f.write("src/a.txt", "edited\n")
	f.mustRun("generate")
	f.assertRuns("generate", 2)
	if got := f.read("gen/out.txt"); got != "edited\n" {
		t.Errorf("output %q", got)
	}
}

func TestCache_BranchSwitchHitsEarlierEntry(t *testing.T) {
	f := newCacheFlake(t)
	f.mustRun("generate")
	f.write("src/a.txt", "B\n")
	f.mustRun("generate")
	// Switching back restores both the source and the committed output.
	f.write("src/a.txt", "A\n")
	f.write("gen/out.txt", "A\n")
	out := f.mustRun("generate")
	if !hit(out, "generate") {
		t.Errorf("returning to A should hit:\n%s", out)
	}
	f.assertRuns("generate", 2)
}

func TestCache_ChangedOutputReruns(t *testing.T) {
	for name, change := range map[string]func(f *cacheFlake){
		"hand-edited": func(f *cacheFlake) { f.write("gen/out.txt", "hand edit\n") },
		"deleted": func(f *cacheFlake) {
			if err := os.Remove(filepath.Join(f.dir, "gen/out.txt")); err != nil {
				f.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCacheFlake(t)
			f.mustRun("generate")
			change(f)
			f.mustRun("generate")
			f.assertRuns("generate", 2)
			if got := f.read("gen/out.txt"); got != "A\n" {
				t.Errorf("output not regenerated: %q", got)
			}
		})
	}
}

func TestCache_FailingTaskRunsAgain(t *testing.T) {
	f := newCacheFlake(t)
	for range 2 {
		stdout, _, err := f.run("run", "fail")
		assertExitCode(t, err, 1)
		if strings.Contains(stdout, "cached") {
			t.Errorf("a failure is never cached:\n%s", stdout)
		}
	}
	f.assertRuns("fail", 2)
}

func TestCache_InterruptedRunIsNotStored(t *testing.T) {
	f := newCacheFlake(t)
	f.write("block", "")
	cmd := exec.Command(f.binary, "run", "slow", "-f", f.dir)
	cmd.Dir = f.dir
	cmd.Env = append(cmd.Environ(), "NIX_TASKS_CACHE_DIR="+f.cacheDir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	waitForFile(t, filepath.Join(f.dir, "slow.started"), time.Minute)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := waitExit(t, cmd, 30*time.Second); code != 130 {
		t.Fatalf("exit code %d, want 130\n%s", code, out.String())
	}

	if err := os.Remove(filepath.Join(f.dir, "block")); err != nil {
		t.Fatal(err)
	}
	f.mustRun("slow")
	f.assertRuns("slow", 2)
}

func TestCache_UpstreamInputChangeRerunsBoth(t *testing.T) {
	f := newCacheFlake(t)
	f.mustRun("package")
	out := f.mustRun("package")
	if !hit(out, "generate") || !hit(out, "package") {
		t.Fatalf("both should hit:\n%s", out)
	}
	f.write("src/a.txt", "edited\n")
	f.mustRun("package")
	f.assertRuns("generate", 2)
	f.assertRuns("package", 2)
	if got := f.read("package.txt"); got != "edited\n" {
		t.Errorf("package.txt %q", got)
	}
}

func TestCache_UncachedUpstreamWarnsAndAlwaysRuns(t *testing.T) {
	f := newCacheFlake(t)
	for range 2 {
		_, stderr, err := f.run("run", "downstream")
		if err != nil {
			t.Fatalf("run failed: %v\n%s", err, stderr)
		}
		want := "warning: downstream has cache enabled but always runs: it depends on tools, which is not cached"
		if !strings.Contains(stripANSI(stderr), want) {
			t.Errorf("missing warning in stderr:\n%s", stderr)
		}
	}
	f.assertRuns("downstream", 2)

	_, stderr, err := f.run("validate")
	if err != nil || !strings.Contains(stripANSI(stderr), "downstream has cache enabled but always runs") {
		t.Errorf("validate should warn: %v\n%s", err, stderr)
	}
}

func TestCache_ForceAndNoCache(t *testing.T) {
	f := newCacheFlake(t)
	f.mustRun("generate")
	if out := f.mustRun("generate", "--force"); strings.Contains(out, "cached") {
		t.Errorf("--force should execute:\n%s", out)
	}
	if out := f.mustRun("generate", "--no-cache"); strings.Contains(out, "cached") {
		t.Errorf("--no-cache should execute:\n%s", out)
	}
	f.assertRuns("generate", 3)
	if out := f.mustRun("generate"); !strings.Contains(out, "cached") {
		t.Errorf("the --force run should have refreshed the entry:\n%s", out)
	}
}

func TestCache_InputPatternMatchingNothingFails(t *testing.T) {
	f := newCacheFlake(t)
	if err := os.Remove(filepath.Join(f.dir, "src/a.txt")); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := f.run("run", "generate")
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout, `cache.inputs pattern "src/*.txt" matches no files`) {
		t.Errorf("missing error:\n%s", stdout)
	}
	f.assertRuns("generate", 0)
}

func TestCache_CleanAndStats(t *testing.T) {
	f := newCacheFlake(t)
	f.mustRun("package")

	stdout, stderr, err := f.run("cache", "stats")
	if err != nil {
		t.Fatalf("cache stats: %v\n%s", err, stderr)
	}
	for _, want := range []string{"generate", "package", "Total: 2 tasks, 2 entries"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stats missing %q:\n%s", want, stdout)
		}
	}

	if _, stderr, err := f.run("cache", "clean", "--task", "generate"); err != nil {
		t.Fatalf("cache clean --task: %v\n%s", err, stderr)
	}
	out := f.mustRun("package")
	if hit(out, "generate") {
		t.Errorf("generate's entries were cleaned:\n%s", out)
	}
	// generate rewrote identical output, so package's key is unchanged.
	if !hit(out, "package") {
		t.Errorf("package should still hit:\n%s", out)
	}

	if stdout, stderr, err := f.run("cache", "clean"); err != nil || !strings.Contains(stdout, "Cache cleared") {
		t.Fatalf("cache clean: %v\n%s%s", err, stdout, stderr)
	}
	if out := f.mustRun("package"); strings.Contains(out, "cached") {
		t.Errorf("nothing should hit after cache clean:\n%s", out)
	}
}

func TestCache_DescribeShowsDeclaration(t *testing.T) {
	f := newCacheFlake(t)
	stdout, stderr, err := f.run("describe", "generate")
	if err != nil {
		t.Fatalf("describe: %v\n%s", err, stderr)
	}
	out := stripANSI(stdout)
	for _, want := range []string{"Cache:", "Inputs:", "- src/*.txt", "Outputs:", "- gen/out.txt", "Max entries: 8"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe missing %q:\n%s", want, out)
		}
	}
	stdout, _, _ = f.run("describe", "tools")
	if !strings.Contains(stripANSI(stdout), "Cache: disabled") {
		t.Errorf("uncached task:\n%s", stdout)
	}
}

func TestCache_RemovedInputsFieldNamesReplacement(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, `{
  tasks.old = tasksLib.mkTask {
    commands = [ "true" ];
    inputs = [ "*.go" ];
  };
}`, map[string]string{})
	_, stderr, code := runNixTasksIn(t, nixTasksBinary(t), dir, "list", "-f", dir)
	if code == 0 || !strings.Contains(stderr, "cache.inputs") {
		t.Errorf("exit %d, stderr should point at cache.inputs:\n%s", code, stderr)
	}
}

func TestCache_BuildUpstreamKeyedByDerivation(t *testing.T) {
	f := newCacheFlake(t)
	_, stderr, err := f.run("run", "use-tool")
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, stderr)
	}
	if strings.Contains(stderr, "always runs") {
		t.Errorf("a build task upstream should not force use-tool to run:\n%s", stderr)
	}
	if out := f.mustRun("use-tool"); !hit(out, "use-tool") {
		t.Errorf("unchanged derivation should hit:\n%s", out)
	}
	f.write("tool.txt", "v2\n")
	if out := f.mustRun("use-tool"); hit(out, "use-tool") {
		t.Errorf("changed derivation should miss:\n%s", out)
	}
	f.assertRuns("use-tool", 2)
}

// A cached task must re-run when one of its packages changes, even though the
// config only carries package names.
func TestRun_CacheInvalidatedByPackageChange(t *testing.T) {
	skipIfNoNix(t)

	config := func(pkg string) string {
		return `{
  packages.tool = pkgs.` + pkg + `;
  tasks.t = { deps = [ "tool" ]; commands = [ "true" ]; cache = { inputs = [ "in.txt" ]; outputs = [ ]; }; };
}`
	}
	dir := writeTempFlake(t, config("hello"), map[string]string{"in.txt": "x"})
	other := writeTempFlake(t, config("jq"), map[string]string{"in.txt": "x"})
	cacheDir := t.TempDir()

	run := func() bool {
		t.Helper()
		r := runJSON(t, dir, cacheDir, "run", "t", "-f", dir)
		r.assertCode(t, 0)
		return r.task(t, "t").Cached
	}

	run()
	if !run() {
		t.Fatal("expected second run to be cached")
	}

	flake, err := os.ReadFile(filepath.Join(other, "flake.nix"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), flake, 0o644); err != nil {
		t.Fatal(err)
	}
	if run() {
		t.Error("expected run after package change to execute")
	}
}
