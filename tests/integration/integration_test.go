//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var builtBinary string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "nix-tasks-integration-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	builtBinary = filepath.Join(dir, "nix-tasks")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", builtBinary, "../../cmd/nix-tasks")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build nix-tasks: %v\n%s", err, output)
		return 1
	}

	return m.Run()
}

// testdataDir returns the path to the testdata directory
func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// nixTasksBinary returns the nix-tasks binary built once by TestMain.
func nixTasksBinary(t *testing.T) string {
	t.Helper()
	return builtBinary
}

// runNixTasks runs nix-tasks with a fresh cache and working directory.
func runNixTasks(t *testing.T, binary string, args ...string) (string, string, error) {
	t.Helper()
	return runNixTasksWithCacheDir(t, binary, t.TempDir(), args...)
}

// runNixTasksWithCacheDir runs nix-tasks with a specific cache directory
func runNixTasksWithCacheDir(t *testing.T, binary, cacheDir string, args ...string) (string, string, error) {
	t.Helper()
	return runNixTasksInDir(t, binary, t.TempDir(), cacheDir, args...)
}

// runNixTasksInDir runs nix-tasks from workDir, which is also where tasks
// run and write files, so tests can inspect what the tasks did.
func runNixTasksInDir(t *testing.T, binary, workDir, cacheDir string, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workDir
	// Tasks run in the flake root, which is shared testdata, so fixtures
	// write their logs to the per-test workDir named here instead.
	cmd.Env = append(testEnviron(), "NIX_TASKS_CACHE_DIR="+cacheDir, "NIX_TASKS_TEST_LOG_DIR="+workDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// testEnviron returns this process's environment without the variables
// GitHub Actions sets to switch nix-tasks to its github output, plus any
// named in drop, so the suite checks the same output wherever it runs.
func testEnviron(drop ...string) []string {
	drop = append(drop, "GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY")
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !slices.Contains(drop, k) {
			env = append(env, kv)
		}
	}
	return env
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	got := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected exit code %d, got non-exit error: %v", want, err)
		}
		got = exitErr.ExitCode()
	}
	if got != want {
		t.Errorf("expected exit code %d, got %d", want, got)
	}
}

// readLines returns the non-empty lines of a file the tasks wrote.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return strings.Fields(string(data))
}

type interval struct{ start, end int64 }

// readTimings parses "<task> start|end <epoch-ms>" lines written by the
// parallel fixture.
func readTimings(t *testing.T, path string) map[string]*interval {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	timings := map[string]*interval{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("malformed timing line %q", line)
		}
		ms, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			t.Fatalf("malformed timestamp in %q: %v", line, err)
		}
		iv := timings[fields[0]]
		if iv == nil {
			iv = &interval{}
			timings[fields[0]] = iv
		}
		if fields[1] == "start" {
			iv.start = ms
		} else {
			iv.end = ms
		}
	}
	return timings
}

// describeSection returns the "- item" entries listed under a describe
// header such as "Depends on:".
func describeSection(stdout, header string) []string {
	var items []string
	inSection := false
	for _, line := range strings.Split(stripANSI(stdout), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		item, ok := strings.CutPrefix(trimmed, "- ")
		if !ok {
			break
		}
		items = append(items, item)
	}
	return items
}

// skipIfNoNix skips the test if nix is not available
func skipIfNoNix(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("nix"); err != nil {
		t.Skip("nix not available, skipping integration test")
	}
}

// =============================================================================
// List Command Tests
// =============================================================================

func TestList_Simple(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "list", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks list failed: %v\nstderr: %s", err, stderr)
	}

	// Check that all tasks are listed
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected 'hello' task in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "goodbye") {
		t.Errorf("expected 'goodbye' task in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "multi-line") {
		t.Errorf("expected 'multi-line' task in output, got: %s", stdout)
	}

	// Check descriptions
	if !strings.Contains(stdout, "Say hello") {
		t.Errorf("expected description 'Say hello' in output, got: %s", stdout)
	}
}

func TestList_WithDependencies(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "list", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks list failed: %v\nstderr: %s", err, stderr)
	}

	// Check tasks are listed
	expectedTasks := []string{"step-a", "step-b", "step-c", "base", "left", "right", "top", "all"}
	for _, task := range expectedTasks {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' task in output, got: %s", task, stdout)
		}
	}
}

// =============================================================================
// Run Command Tests - Basic
// =============================================================================

func TestRun_SimpleTask(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "hello")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"hello": "succeeded"})
	if r.Doc.Counts.Passed != 1 {
		t.Errorf("counts %+v", r.Doc.Counts)
	}
}

func TestRun_TaskNotFound(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	_, stderr, err := runNixTasks(t, binary, "run", "nonexistent", "-f", flakePath)
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}

	if !strings.Contains(stderr, "task not found") {
		t.Errorf("expected 'task not found' error, got: %s", stderr)
	}
}

func TestRun_VerboseOutput(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "run", "hello", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "[hello] Hello, World!") {
		t.Errorf("expected prefixed '[hello] Hello, World!' in verbose output, got: %s", stdout)
	}
}

// =============================================================================
// Run Command Tests - Dependencies
// =============================================================================

func TestRun_LinearDependencyChain(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "step-c", "-f", flakePath)
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"step-a": "succeeded", "step-b": "succeeded", "step-c": "succeeded"})
	if !slices.Equal(r.names(), []string{"step-a", "step-b", "step-c"}) {
		t.Errorf("tasks listed as %v, want dependencies first", r.names())
	}
	if !r.finishedBefore(t, "step-a", "step-b") || !r.finishedBefore(t, "step-b", "step-c") {
		t.Error("each step should start after the one it depends on finishes")
	}

	order := readLines(t, filepath.Join(workDir, "order.log"))
	if !slices.Equal(order, []string{"step-a", "step-b", "step-c"}) {
		t.Errorf("expected tasks to run in order [step-a step-b step-c], got %v", order)
	}
}

func TestRun_DiamondDependency(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "top", "-f", flakePath)
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"base": "succeeded", "left": "succeeded", "right": "succeeded", "top": "succeeded"})
	for _, edge := range [][2]string{{"base", "left"}, {"base", "right"}, {"left", "top"}, {"right", "top"}} {
		if !r.finishedBefore(t, edge[0], edge[1]) {
			t.Errorf("%s should finish before %s starts", edge[0], edge[1])
		}
	}

	order := readLines(t, filepath.Join(workDir, "order.log"))
	if len(order) != 4 {
		t.Fatalf("expected each of the 4 tasks to run exactly once, got %v", order)
	}
}

func TestRun_CompoundTask(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "all", "-f", flakePath)
	r.assertCode(t, 0)
	want := map[string]string{"all": "succeeded"}
	for _, task := range []string{"step-a", "step-b", "step-c", "base", "left", "right", "top"} {
		want[task] = "succeeded"
	}
	r.assertStates(t, want)
	if names := r.names(); names[len(names)-1] != "all" {
		t.Errorf("the compound task should be listed after everything it groups: %v", names)
	}

	order := readLines(t, filepath.Join(workDir, "order.log"))
	if len(order) != 7 {
		t.Errorf("expected each of the 7 leaf tasks to run exactly once, got %v", order)
	}
}

// =============================================================================
// Run Command Tests - Parallel Execution
// =============================================================================

func TestRun_ParallelExecution(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "final", "-f", flakePath, "-j", "4")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{
		"task-1": "succeeded", "task-2": "succeeded", "task-3": "succeeded", "task-4": "succeeded", "final": "succeeded",
	})
	for _, task := range []string{"task-1", "task-2", "task-3", "task-4"} {
		if !r.finishedBefore(t, task, "final") {
			t.Errorf("final should start after %s finishes", task)
		}
	}

	timings := readTimings(t, filepath.Join(workDir, "timings.log"))
	if len(timings) != 4 {
		t.Fatalf("expected timings for 4 tasks, got %v", timings)
	}
	var latestStart, earliestEnd int64 = 0, 1<<63 - 1
	for _, iv := range timings {
		latestStart = max(latestStart, iv.start)
		earliestEnd = min(earliestEnd, iv.end)
	}
	if latestStart >= earliestEnd {
		t.Errorf("expected all 4 tasks to overlap with -j 4: last start %d >= first end %d", latestStart, earliestEnd)
	}
}

func TestRun_SingleJob(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "final", "-f", flakePath, "-j", "1")
	r.assertCode(t, 0)
	if r.Doc.Counts.Passed != 5 {
		t.Errorf("expected 5 passed tasks, got %+v", r.Doc.Counts)
	}

	timings := readTimings(t, filepath.Join(workDir, "timings.log"))
	intervals := make([]interval, 0, len(timings))
	for _, iv := range timings {
		intervals = append(intervals, *iv)
	}
	slices.SortFunc(intervals, func(a, b interval) int { return int(a.start - b.start) })
	for i := 1; i < len(intervals); i++ {
		if intervals[i].start < intervals[i-1].end {
			t.Errorf("expected no overlap with -j 1, got intervals %v", intervals)
			break
		}
	}
}

// =============================================================================
// Run Command Tests - Error Handling
// =============================================================================

func TestRun_FailingTask(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "failing")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "fail")
	r.assertCode(t, 1)
	r.assertStates(t, map[string]string{"fail": "failed"})
	if fail := r.task(t, "fail"); fail.ExitCode == nil || *fail.ExitCode != 1 || !strings.Contains(r.log(t, "fail"), "About to fail...") {
		t.Errorf("failed task should have exit code 1 and its output in its log: %+v", fail)
	}

	// Failed tasks show their output in the summary even without -v.
	stdout, _, err := runNixTasks(t, nixTasksBinary(t), "run", "fail", "-f", flakePath)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout, "About to fail...") {
		t.Errorf("expected failing task output in summary, got: %s", stdout)
	}
}

func TestRun_ContinueOnError(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	// Task with continueOnError should allow dependent tasks to run
	r := runJSON(t, workDir, t.TempDir(), "run", "after-continue", "-f", flakePath)
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"fail-continue": "failed_allowed", "after-continue": "succeeded"})
	if !strings.Contains(r.log(t, "fail-continue"), "This will fail but continue...") {
		t.Error("the allowed failure's output should be in its log")
	}
	if r.Doc.Counts.AllowedFailures != 1 {
		t.Errorf("counts %+v", r.Doc.Counts)
	}

	ran := readLines(t, filepath.Join(workDir, "ran.log"))
	if !slices.Equal(ran, []string{"after-continue"}) {
		t.Errorf("expected after-continue to run, got %v", ran)
	}
}

func TestRun_ContinueOnErrorFlag(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "parallel-test", "-f", flakePath, "--continue-on-error")
	r.assertCode(t, 1)
	for name, want := range map[string]string{"independent-a": "succeeded", "independent-b": "failed", "independent-c": "succeeded"} {
		if got := r.task(t, name).State; got != want {
			t.Errorf("task %s: state %q, want %q", name, got, want)
		}
	}

	ran := readLines(t, filepath.Join(workDir, "ran.log"))
	for _, name := range []string{"independent-a", "independent-c"} {
		if !slices.Contains(ran, name) {
			t.Errorf("expected %s to run, ran: %v", name, ran)
		}
	}
}

func TestRun_ContinueOnErrorFlagSkipsDependents(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "parallel-test", "-f", flakePath, "--continue-on-error")
	r.assertCode(t, 1)
	r.assertStates(t, map[string]string{
		"independent-a": "succeeded", "independent-b": "failed", "independent-c": "succeeded", "parallel-test": "skipped",
	})
	if reason := r.task(t, "parallel-test").SkipReason; reason == nil || *reason != "independent-b failed" {
		t.Errorf("skip reason %v, want the failed dependency", reason)
	}
	if ran := readLines(t, filepath.Join(workDir, "ran.log")); slices.Contains(ran, "parallel-test") {
		t.Errorf("parallel-test depends on a failed task and should not run, ran: %v", ran)
	}
}

func TestRun_FailFast(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	r := runJSON(t, workDir, t.TempDir(), "run", "after-fail", "-f", flakePath)
	r.assertCode(t, 1)
	r.assertStates(t, map[string]string{"fail": "failed", "after-fail": "skipped"})
	if r.Doc.Counts.Skipped != 1 {
		t.Errorf("counts %+v", r.Doc.Counts)
	}
	if reason := r.task(t, "after-fail").SkipReason; reason == nil || *reason != "fail failed" {
		t.Errorf("skip reason %v", reason)
	}
	if _, err := os.Stat(filepath.Join(workDir, "ran.log")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after-fail should not have run, ran.log stat: %v", err)
	}
}

// =============================================================================
// Run Command Tests - Cycle Detection
// =============================================================================

func TestRun_CycleDetection(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "cycle")

	_, stderr, err := runNixTasks(t, binary, "run", "task-a", "-f", flakePath)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}

	if !strings.Contains(stderr, "circular dependency") {
		t.Errorf("expected 'circular dependency' error, got: %s", stderr)
	}
}

// =============================================================================
// Run Command Tests - Environment Variables
// =============================================================================

func TestRun_EnvironmentVariables(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "env")

	stdout, stderr, err := runNixTasks(t, binary, "run", "print-env", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Check environment variables are set
	if !strings.Contains(stdout, "MY_VAR=hello") {
		t.Errorf("expected 'MY_VAR=hello' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "ANOTHER_VAR=world") {
		t.Errorf("expected 'ANOTHER_VAR=world' in output, got: %s", stdout)
	}
}

func TestRun_EnvironmentCheck(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "env")

	stdout, stderr, err := runNixTasks(t, binary, "run", "check-env", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Environment check passed") {
		t.Errorf("expected 'Environment check passed' in output, got: %s", stdout)
	}
}

// =============================================================================
// Describe Command Tests
// =============================================================================

func TestDescribe_TaskWithDependencies(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "top", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stripANSI(stdout), "Task: top") {
		t.Errorf("expected 'Task: top' in output, got: %s", stdout)
	}
	if deps := describeSection(stdout, "Depends on:"); !slices.Equal(deps, []string{"left", "right"}) {
		t.Errorf("expected dependencies [left right], got %v\noutput: %s", deps, stdout)
	}
	if dependents := describeSection(stdout, "Depended on by:"); !slices.Equal(dependents, []string{"all"}) {
		t.Errorf("expected dependents [all], got %v\noutput: %s", dependents, stdout)
	}
}

func TestDescribe_TaskDependedOnBy(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "base", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	if dependents := describeSection(stdout, "Depended on by:"); !slices.Equal(dependents, []string{"left", "right"}) {
		t.Errorf("expected dependents [left right], got %v\noutput: %s", dependents, stdout)
	}
	if deps := describeSection(stdout, "Depends on:"); len(deps) != 0 {
		t.Errorf("expected no dependencies, got %v", deps)
	}
}

func TestDescribe_TaskNotFound(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	_, stderr, err := runNixTasks(t, binary, "describe", "nonexistent", "-f", flakePath)
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}

	if !strings.Contains(stderr, "task not found") {
		t.Errorf("expected 'task not found' error, got: %s", stderr)
	}
}

// =============================================================================
// Validate Command Tests
// =============================================================================

func TestValidate_ValidConfig(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "validate", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks validate failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Configuration is valid") {
		t.Errorf("expected 'Configuration is valid' in output, got: %s", stdout)
	}
}

func TestValidate_ShowsStats(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "validate", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks validate failed: %v\nstderr: %s", err, stderr)
	}

	plain := stripANSI(stdout)
	for _, want := range []string{"Tasks: 8", "Packages: 0", "Dev Shells: 1"} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected %q in output, got: %s", want, stdout)
		}
	}
}

// =============================================================================
// Shell Tests
// =============================================================================

func TestRun_TaskWithPackageDeps(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	stdout, stderr, err := runNixTasks(t, binary, "run", "check-jq", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !regexp.MustCompile(`(?m)^\[check-jq\] jq-\d`).MatchString(stdout) {
		t.Errorf("expected jq version from the task, got: %s", stdout)
	}
}

func TestShell_CommandFlag(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	stdout, stderr, err := runNixTasks(t, binary, "shell", "minimal", "-f", flakePath, "-c", "command -v jq")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	assertNixStorePath(t, stdout, "jq")
}

func TestShell_DefaultShell(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	stdout, stderr, err := runNixTasks(t, binary, "shell", "-f", flakePath, "-c", "echo SHELL_TYPE=$SHELL_TYPE")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	// default sets no SHELL_TYPE of its own, so it inherits extended's override of minimal's.
	if !strings.Contains(stdout, "SHELL_TYPE=extended\n") {
		t.Errorf("expected SHELL_TYPE=extended in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Welcome to the test shell") {
		t.Errorf("expected default shell's shellHook to run, got: %s", stdout)
	}
}

func TestShell_InheritedPackages(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	// The 'extended' shell should have both jq (from minimal) and curl
	stdout, stderr, err := runNixTasks(t, binary, "shell", "extended", "-f", flakePath, "-c", "command -v jq && command -v curl")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	assertNixStorePath(t, stdout, "jq")
	assertNixStorePath(t, stdout, "curl")
}

// assertNixStorePath checks that output contains a line resolving tool to a
// /nix/store path, so a host-installed copy can't satisfy the test.
func assertNixStorePath(t *testing.T, stdout, tool string) {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "/bin/"+tool) {
			if !strings.HasPrefix(line, "/nix/store/") {
				t.Errorf("expected %s from /nix/store, got %s", tool, line)
			}
			return
		}
	}
	t.Errorf("expected a resolved path for %s, got: %s", tool, stdout)
}

func TestShell_NotFound(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	_, stderr, err := runNixTasks(t, binary, "shell", "nonexistent", "-f", flakePath, "-c", "echo test")
	if err == nil {
		t.Fatal("expected error for nonexistent shell")
	}

	if !strings.Contains(stderr, "shell not found") {
		t.Errorf("expected 'shell not found' error, got: %s", stderr)
	}
}

func TestList_ShowsShells(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	stdout, stderr, err := runNixTasks(t, binary, "list", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks list failed: %v\nstderr: %s", err, stderr)
	}

	// Should show shells section
	if !strings.Contains(stdout, "Dev Shells:") {
		t.Errorf("expected 'Dev Shells:' in output, got: %s", stdout)
	}

	// Should show all shells
	if !strings.Contains(stdout, "minimal") {
		t.Errorf("expected 'minimal' shell in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "extended") {
		t.Errorf("expected 'extended' shell in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "default") {
		t.Errorf("expected 'default' shell in output, got: %s", stdout)
	}

	// Should show inheritance info
	if !strings.Contains(stdout, "extends minimal") {
		t.Errorf("expected 'extends minimal' in output, got: %s", stdout)
	}
}

// =============================================================================
// Duration Tests
// =============================================================================

func TestRun_ShowsDuration(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "run", "goodbye", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !regexp.MustCompile(`(?m)^✓ goodbye \(\d+(\.\d+)?(ms|s)\)$`).MatchString(stripANSI(stdout)) {
		t.Errorf("expected 'goodbye' with a duration like (123ms), got: %s", stdout)
	}
}

// =============================================================================
// Stream Flag Tests
// =============================================================================

func TestRun_StreamFlag(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "run", "multi-line", "-f", flakePath, "--stream")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "[multi-line] Line 1\n[multi-line] Line 2\n[multi-line] Line 3\n") {
		t.Errorf("expected prefixed, ordered streaming output, got: %s", stdout)
	}
}

// =============================================================================
// mkGoTask Builder Tests
// =============================================================================

func TestMkGoTask_BuildsGoApplication(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")

	r := runJSON(t, t.TempDir(), t.TempDir(), "run", "build", "-f", flakePath)
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"build": "succeeded"})

	// Outputs are linked in the flake root, not the directory nix-tasks ran in.
	link := filepath.Join(flakePath, ".nix-tasks", "build", "out")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("expected output link at %s: %v", link, err)
	}
	if !strings.HasPrefix(target, "/nix/store/") {
		t.Errorf("expected output link into /nix/store, got %s", target)
	}
	if _, err := os.Stat(filepath.Join(link, "bin", "testapp")); err != nil {
		t.Errorf("expected built binary behind output link: %v", err)
	}
}

func TestMkGoTask_BuildsBinaryToCorrectLocation(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "test")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"build": "succeeded", "test": "succeeded"})
	if !strings.Contains(r.log(t, "test"), "Hello from mkGoTask!") {
		t.Error("expected 'Hello from mkGoTask!' from the built binary in test's log")
	}
}

// =============================================================================
// Module Composition Tests
// =============================================================================

func TestRun_ModifiedCompoundTask(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "publish")
	r.assertCode(t, 0)
	// Compound task deps are siblings with no ordering between them, so only
	// check that the prepended one was pulled in.
	r.assertStates(t, map[string]string{
		"helm-set-app-version": "succeeded", "ko-publish": "succeeded", "helm-publish": "succeeded", "publish": "succeeded",
	})
	if !strings.Contains(r.log(t, "helm-set-app-version"), "Setting app version") {
		t.Error("expected the prepended task's output in its log")
	}
}

func TestRun_AppendedTaskDeps(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "publish-with-cleanup")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{
		"ko-publish": "succeeded", "helm-publish": "succeeded", "cleanup": "succeeded", "publish-with-cleanup": "succeeded",
	})
}

func TestRun_PipedModifications(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "build")
	r.assertCode(t, 0)
	if got, want := r.log(t, "build"), "Pre-build step\nBuilding...\nPost-build step\n"; !strings.Contains(got, want) {
		t.Errorf("expected prepended, original and appended commands in order, log holds: %q", got)
	}
}

func TestDescribe_ModifiedCompoundTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "publish", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	want := []string{"helm-set-app-version", "ko-publish", "helm-publish"}
	if deps := describeSection(stdout, "Depends on:"); !slices.Equal(deps, want) {
		t.Errorf("expected dependencies %v, got %v\noutput: %s", want, deps, stdout)
	}
}

func TestDescribe_PipedModifiedTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "build", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	// Should show modified description
	if !strings.Contains(stdout, "Build the app (customized)") {
		t.Errorf("expected modified description, got: %s", stdout)
	}
}

func TestRun_ScriptWithPrependedAndAppendedCommands(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "composition")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "script-modified")
	r.assertCode(t, 0)
	log := r.log(t, "script-modified")

	// Prepended commands, the original script and appended commands, in order.
	beforePos := strings.Index(log, "Before script")
	scriptPos := strings.Index(log, "Script line 1")
	afterPos := strings.Index(log, "After script")
	if beforePos < 0 || scriptPos < 0 || afterPos < 0 || beforePos > scriptPos || scriptPos > afterPos {
		t.Errorf("commands missing or out of order: before=%d, script=%d, after=%d\nlog: %s", beforePos, scriptPos, afterPos, log)
	}
}

// =============================================================================
// After-Hooks Tests
// =============================================================================

func TestList_AfterHooks(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "list", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks list failed: %v\nstderr: %s", err, stderr)
	}

	expectedTasks := []string{"build", "post-build", "unrelated", "chain-a", "chain-b", "chain-c", "deploy", "post-deploy", "setup", "test", "coverage-report", "notify"}
	for _, task := range expectedTasks {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' task in output, got: %s", task, stdout)
		}
	}
}

func TestRun_BasicAfterHook(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "build")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"build": "succeeded", "post-build": "succeeded"})
	if !r.finishedBefore(t, "build", "post-build") {
		t.Error("post-build should start after build finishes")
	}
	if !r.task(t, "post-build").AfterHook || r.task(t, "build").AfterHook {
		t.Error("post-build should be the run's only after-hook")
	}
}

func TestRun_AfterHookNotTriggered(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "unrelated")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"unrelated": "succeeded"})
}

func TestRun_TransitiveAfterHooks(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "chain-a")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"chain-a": "succeeded", "chain-b": "succeeded", "chain-c": "succeeded"})
	if !r.finishedBefore(t, "chain-a", "chain-b") || !r.finishedBefore(t, "chain-b", "chain-c") {
		t.Error("each hook should start after the task it follows finishes")
	}
}

func TestRun_AfterHookWithDepends(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	// post-deploy is pulled in via after, and brings its own dependency setup
	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "deploy")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"setup": "succeeded", "deploy": "succeeded", "post-deploy": "succeeded"})
	if !r.finishedBefore(t, "deploy", "post-deploy") {
		t.Error("post-deploy should start after deploy finishes")
	}
	if !r.finishedBefore(t, "setup", "post-deploy") {
		t.Error("post-deploy should start after setup finishes")
	}
	// setup is pulled in by the hook, but as its dependency, not as a hook.
	for name, want := range map[string]bool{"deploy": false, "post-deploy": true, "setup": false} {
		if got := r.task(t, name).AfterHook; got != want {
			t.Errorf("%s: afterHook %v, want %v", name, got, want)
		}
	}
}

func TestRun_MultipleAfterHooksOnSameTarget(t *testing.T) {
	skipIfNoNix(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	r := runFlakeJSON(t, flakePath, t.TempDir(), "run", "test")
	r.assertCode(t, 0)
	r.assertStates(t, map[string]string{"test": "succeeded", "coverage-report": "succeeded", "notify": "succeeded"})
}

func TestDescribe_AfterHook(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "post-build", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	if after := describeSection(stdout, "Runs after:"); !slices.Equal(after, []string{"build"}) {
		t.Errorf("expected 'Runs after:' [build], got %v\noutput: %s", after, stdout)
	}
}

func TestDescribe_AfterHookTarget(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "describe", "build", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks describe failed: %v\nstderr: %s", err, stderr)
	}

	if hooks := describeSection(stdout, "After-hooked by:"); !slices.Equal(hooks, []string{"post-build"}) {
		t.Errorf("expected 'After-hooked by:' [post-build], got %v\noutput: %s", hooks, stdout)
	}
}
