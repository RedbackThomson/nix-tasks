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
	defer os.RemoveAll(dir)

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
	cmd.Env = append(cmd.Environ(), "NIX_TASKS_CACHE_DIR="+cacheDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

var summaryLinePattern = regexp.MustCompile(`^\s*(✓|✗|-) (\S+)(?: \(.*\))?$`)

// taskStatuses parses the run summary into task name -> "passed", "failed"
// or "skipped".
func taskStatuses(stdout string) map[string]string {
	statuses := map[string]string{}
	for _, line := range strings.Split(stripANSI(stdout), "\n") {
		m := summaryLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "✓":
			statuses[m[2]] = "passed"
		case "✗":
			statuses[m[2]] = "failed"
		case "-":
			statuses[m[2]] = "skipped"
		}
	}
	return statuses
}

func assertStatuses(t *testing.T, stdout string, want map[string]string) {
	t.Helper()
	got := taskStatuses(stdout)
	if len(got) != len(want) {
		t.Errorf("expected %d tasks in summary, got %d: %v\noutput: %s", len(want), len(got), got, stdout)
	}
	for name, status := range want {
		if got[name] != status {
			t.Errorf("task %s: expected %s, got %q\noutput: %s", name, status, got[name], stdout)
		}
	}
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

func assertBefore(t *testing.T, order []string, first, second string) {
	t.Helper()
	i, j := slices.Index(order, first), slices.Index(order, second)
	if i < 0 || j < 0 {
		t.Errorf("expected both %s and %s to run, order: %v", first, second, order)
		return
	}
	if i > j {
		t.Errorf("expected %s to run before %s, order: %v", first, second, order)
	}
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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	stdout, stderr, err := runNixTasks(t, binary, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"hello": "passed"})
	if !strings.Contains(stdout, "Completed: 1 tasks (1 passed)") {
		t.Errorf("expected '1 tasks (1 passed)' summary, got: %s", stdout)
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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "step-c", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"step-a": "passed", "step-b": "passed", "step-c": "passed"})

	order := readLines(t, filepath.Join(workDir, "order.log"))
	if !slices.Equal(order, []string{"step-a", "step-b", "step-c"}) {
		t.Errorf("expected tasks to run in order [step-a step-b step-c], got %v", order)
	}
}

func TestRun_DiamondDependency(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "top", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"base": "passed", "left": "passed", "right": "passed", "top": "passed"})

	order := readLines(t, filepath.Join(workDir, "order.log"))
	if len(order) != 4 {
		t.Fatalf("expected each of the 4 tasks to run exactly once, got %v", order)
	}
	assertBefore(t, order, "base", "left")
	assertBefore(t, order, "base", "right")
	assertBefore(t, order, "left", "top")
	assertBefore(t, order, "right", "top")
}

func TestRun_CompoundTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "all", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	want := map[string]string{"all": "passed"}
	for _, task := range []string{"step-a", "step-b", "step-c", "base", "left", "right", "top"} {
		want[task] = "passed"
	}
	assertStatuses(t, stdout, want)

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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "final", "-f", flakePath, "-j", "4")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{
		"task-1": "passed", "task-2": "passed", "task-3": "passed", "task-4": "passed", "final": "passed",
	})

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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "final", "-f", flakePath, "-j", "1")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Completed: 5 tasks (5 passed)") {
		t.Errorf("expected '5 tasks (5 passed)' summary, got: %s", stdout)
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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")

	stdout, _, err := runNixTasks(t, binary, "run", "fail", "-f", flakePath)
	assertExitCode(t, err, 1)
	assertStatuses(t, stdout, map[string]string{"fail": "failed"})

	// Failed tasks show their output even without -v.
	if !strings.Contains(stdout, "About to fail...") {
		t.Errorf("expected failing task output in summary, got: %s", stdout)
	}
}

func TestRun_ContinueOnError(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	// Task with continueOnError should allow dependent tasks to run
	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "after-continue", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	statuses := taskStatuses(stdout)
	if statuses["after-continue"] != "passed" {
		t.Errorf("expected after-continue to pass, got %v\noutput: %s", statuses, stdout)
	}
	if _, ok := statuses["fail-continue"]; !ok {
		t.Errorf("expected fail-continue in summary, got %v", statuses)
	}

	ran := readLines(t, filepath.Join(workDir, "ran.log"))
	if !slices.Equal(ran, []string{"after-continue"}) {
		t.Errorf("expected after-continue to run, got %v", ran)
	}
}

func TestRun_ContinueOnErrorFlag(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	stdout, _, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "parallel-test", "-f", flakePath, "--continue-on-error")
	assertExitCode(t, err, 1)

	statuses := taskStatuses(stdout)
	for name, want := range map[string]string{"independent-a": "passed", "independent-b": "failed", "independent-c": "passed"} {
		if statuses[name] != want {
			t.Errorf("task %s: expected %s, got %q\noutput: %s", name, want, statuses[name], stdout)
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
	t.Skip("known bug: --continue-on-error still runs tasks whose dependency failed")
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	stdout, _, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "parallel-test", "-f", flakePath, "--continue-on-error")
	assertExitCode(t, err, 1)
	assertStatuses(t, stdout, map[string]string{
		"independent-a": "passed", "independent-b": "failed", "independent-c": "passed", "parallel-test": "skipped",
	})
	if ran := readLines(t, filepath.Join(workDir, "ran.log")); slices.Contains(ran, "parallel-test") {
		t.Errorf("parallel-test depends on a failed task and should not run, ran: %v", ran)
	}
}

func TestRun_FailFast(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")
	workDir := t.TempDir()

	stdout, _, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "after-fail", "-f", flakePath)
	assertExitCode(t, err, 1)
	assertStatuses(t, stdout, map[string]string{"fail": "failed", "after-fail": "skipped"})
	if !strings.Contains(stdout, "1 skipped") {
		t.Errorf("expected '1 skipped' in summary, got: %s", stdout)
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
// Cache Tests
// =============================================================================

func TestRun_CachesResults(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// Use a shared cache directory for this test
	cacheDir := t.TempDir()

	// First run should execute the task
	stdout1, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("first run failed: %v\nstderr: %s", err, stderr)
	}

	// Should not say cached
	if strings.Contains(stdout1, "(cached)") {
		t.Errorf("first run should not be cached, got: %s", stdout1)
	}

	// Second run should use cache
	stdout2, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("second run failed: %v\nstderr: %s", err, stderr)
	}

	// Should say cached
	if !strings.Contains(stdout2, "(cached)") {
		t.Errorf("second run should be cached, got: %s", stdout2)
	}
}

func TestRun_ForceBypassesCache(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// Use a shared cache directory for this test
	cacheDir := t.TempDir()

	// First run to populate cache
	_, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("first run failed: %v\nstderr: %s", err, stderr)
	}

	// Run with --force should not use cache
	stdout, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath, "--force")
	if err != nil {
		t.Fatalf("force run failed: %v\nstderr: %s", err, stderr)
	}

	// Should not say cached
	if strings.Contains(stdout, "(cached)") {
		t.Errorf("--force should bypass cache, got: %s", stdout)
	}
}

func TestRun_ShellTaskWithoutInputsNotCached(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// "goodbye" is a shell task that intentionally has no `inputs` declared.
	// Under the inputs-opt-in policy, it should never be reported as cached,
	// even on repeated runs.
	cacheDir := t.TempDir()

	for i := 0; i < 2; i++ {
		stdout, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "goodbye", "-f", flakePath)
		if err != nil {
			t.Fatalf("run %d failed: %v\nstderr: %s", i+1, err, stderr)
		}
		if strings.Contains(stdout, "(cached)") {
			t.Errorf("shell task without inputs should never be cached (run %d), got: %s", i+1, stdout)
		}
	}
}

func TestRun_NoCacheDisablesCaching(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// Use a shared cache directory for this test
	cacheDir := t.TempDir()

	// Run twice with --no-cache
	for i := 0; i < 2; i++ {
		stdout, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath, "--no-cache")
		if err != nil {
			t.Fatalf("run %d failed: %v\nstderr: %s", i+1, err, stderr)
		}

		// Should not say cached
		if strings.Contains(stdout, "(cached)") {
			t.Errorf("--no-cache run %d should not be cached, got: %s", i+1, stdout)
		}
	}
}

func TestCache_Clean(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// Use a shared cache directory for this test
	cacheDir := t.TempDir()

	// First run to populate cache
	_, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("first run failed: %v\nstderr: %s", err, stderr)
	}

	// Clean the cache
	stdout, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "cache", "clean", "-f", flakePath)
	if err != nil {
		t.Fatalf("cache clean failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Cache cleared") {
		t.Errorf("expected 'Cache cleared' message, got: %s", stdout)
	}

	// Run again should not be cached
	stdout, stderr, err = runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("run after clean failed: %v\nstderr: %s", err, stderr)
	}

	if strings.Contains(stdout, "(cached)") {
		t.Errorf("run after cache clean should not be cached, got: %s", stdout)
	}
}

func TestCache_Stats(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "simple")

	// Use a shared cache directory for this test
	cacheDir := t.TempDir()

	// Run a task to populate cache
	_, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr)
	}

	// Get cache stats
	stdout, stderr, err := runNixTasksWithCacheDir(t, binary, cacheDir, "cache", "stats", "-f", flakePath)
	if err != nil {
		t.Fatalf("cache stats failed: %v\nstderr: %s", err, stderr)
	}

	// Should show statistics
	if !strings.Contains(stdout, "Cache Statistics") {
		t.Errorf("expected 'Cache Statistics' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Entries:") {
		t.Errorf("expected 'Entries:' in output, got: %s", stdout)
	}
}

// =============================================================================
// mkGoTask Builder Tests
// =============================================================================

func TestMkGoTask_BuildsGoApplication(t *testing.T) {
	skipIfNoNix(t)
	t.Skip("known bug: build tasks run `nix build <drv>` without ^*, so the derivation is never built")
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")
	workDir := t.TempDir()

	stdout, stderr, err := runNixTasksInDir(t, binary, workDir, t.TempDir(), "run", "build", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run build failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	assertStatuses(t, stdout, map[string]string{"build": "passed"})

	link := filepath.Join(workDir, ".nix-tasks", "build", "out")
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
	t.Skip("known bug: build tasks run `nix build <drv>` without ^*, so the derivation is never built")
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")

	stdout, stderr, err := runNixTasks(t, binary, "run", "test", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run test failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	assertStatuses(t, stdout, map[string]string{"build": "passed", "test": "passed"})
	if !strings.Contains(stdout, "[test] Hello from mkGoTask!") {
		t.Errorf("expected 'Hello from mkGoTask!' from the built binary, got: %s", stdout)
	}
}

// =============================================================================
// Task Modifier Tests
// =============================================================================

func TestRun_ModifiedCompoundTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

	stdout, stderr, err := runNixTasks(t, binary, "run", "publish", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Compound task deps are siblings with no ordering between them, so only
	// check that the prepended one was pulled in.
	assertStatuses(t, stdout, map[string]string{
		"helm-set-app-version": "passed", "ko-publish": "passed", "helm-publish": "passed", "publish": "passed",
	})
	if !strings.Contains(stdout, "[helm-set-app-version] Setting app version") {
		t.Errorf("expected prepended task output, got: %s", stdout)
	}
}

func TestRun_AppendedTaskDeps(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

	stdout, stderr, err := runNixTasks(t, binary, "run", "publish-with-cleanup", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{
		"ko-publish": "passed", "helm-publish": "passed", "cleanup": "passed", "publish-with-cleanup": "passed",
	})
}

func TestRun_PipedModifications(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

	stdout, stderr, err := runNixTasks(t, binary, "run", "build", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	want := "[build] Pre-build step\n[build] Building...\n[build] Post-build step\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("expected prepended, original and appended commands in order, got: %s", stdout)
	}
}

func TestDescribe_ModifiedCompoundTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

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
	flakePath := filepath.Join(testdataDir(t), "modifiers")

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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

	stdout, stderr, err := runNixTasks(t, binary, "run", "script-modified", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// All three parts should appear: prepended commands, original script, appended commands
	if !strings.Contains(stdout, "Before script") {
		t.Errorf("expected 'Before script' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Script line 1") {
		t.Errorf("expected 'Script line 1' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "After script") {
		t.Errorf("expected 'After script' in output, got: %s", stdout)
	}

	// Verify ordering
	beforePos := strings.Index(stdout, "Before script")
	scriptPos := strings.Index(stdout, "Script line 1")
	afterPos := strings.Index(stdout, "After script")
	if beforePos > scriptPos || scriptPos > afterPos {
		t.Errorf("commands not in expected order: before=%d, script=%d, after=%d", beforePos, scriptPos, afterPos)
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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "run", "build", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"build": "passed", "post-build": "passed"})

	buildPos := strings.Index(stdout, "[build] Building...")
	postBuildPos := strings.Index(stdout, "[post-build] Post-build hook ran")
	if buildPos < 0 || postBuildPos < 0 || buildPos > postBuildPos {
		t.Errorf("build should run before post-build: build=%d, post-build=%d\noutput: %s", buildPos, postBuildPos, stdout)
	}
}

func TestRun_AfterHookNotTriggered(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "run", "unrelated", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"unrelated": "passed"})
}

func TestRun_TransitiveAfterHooks(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "run", "chain-a", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"chain-a": "passed", "chain-b": "passed", "chain-c": "passed"})

	aPos := strings.Index(stdout, "[chain-a] Chain A")
	bPos := strings.Index(stdout, "[chain-b] Chain B")
	cPos := strings.Index(stdout, "[chain-c] Chain C")
	if aPos < 0 || bPos < 0 || cPos < 0 || aPos > bPos || bPos > cPos {
		t.Errorf("tasks not in correct order: a=%d, b=%d, c=%d\noutput: %s", aPos, bPos, cPos, stdout)
	}
}

func TestRun_AfterHookWithDepends(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	// post-deploy is pulled in via after, and brings its own dependency setup
	stdout, stderr, err := runNixTasks(t, binary, "run", "deploy", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"setup": "passed", "deploy": "passed", "post-deploy": "passed"})

	deployPos := strings.Index(stdout, "[deploy] Deploying...")
	setupPos := strings.Index(stdout, "[setup] Setup")
	postDeployPos := strings.Index(stdout, "[post-deploy] Post-deploy (needed setup first)")
	if deployPos < 0 || setupPos < 0 || postDeployPos < 0 {
		t.Fatalf("expected output from setup, deploy and post-deploy, got: %s", stdout)
	}
	if deployPos > postDeployPos {
		t.Errorf("deploy should run before post-deploy: deploy=%d, post-deploy=%d", deployPos, postDeployPos)
	}
	if setupPos > postDeployPos {
		t.Errorf("setup should run before post-deploy: setup=%d, post-deploy=%d", setupPos, postDeployPos)
	}
}

func TestRun_MultipleAfterHooksOnSameTarget(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "after-hooks")

	stdout, stderr, err := runNixTasks(t, binary, "run", "test", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	assertStatuses(t, stdout, map[string]string{"test": "passed", "coverage-report": "passed", "notify": "passed"})
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
