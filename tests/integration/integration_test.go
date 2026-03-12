//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testdataDir returns the path to the testdata directory
func testdataDir(t *testing.T) string {
	t.Helper()
	// Get the directory of this test file using runtime.Caller
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// nixTasksBinary builds and returns the path to the nix-tasks binary
func nixTasksBinary(t *testing.T) string {
	t.Helper()

	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "nix-tasks")

	cmd := exec.Command("go", "build", "-o", binary, "../../cmd/nix-tasks")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build nix-tasks: %v\n%s", err, output)
	}

	return binary
}

// runNixTasks runs nix-tasks with the given arguments and returns stdout, stderr, and error
// Each call gets a fresh cache directory to avoid test interference
func runNixTasks(t *testing.T, binary string, args ...string) (string, string, error) {
	t.Helper()
	return runNixTasksWithCacheDir(t, binary, t.TempDir(), args...)
}

// runNixTasksWithCacheDir runs nix-tasks with a specific cache directory
func runNixTasksWithCacheDir(t *testing.T, binary, cacheDir string, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(cmd.Environ(), "NIX_TASKS_CACHE_DIR="+cacheDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
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

	// Check success indicator
	if !strings.Contains(stdout, "✓") {
		t.Errorf("expected success checkmark in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected 'hello' task name in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 tasks") {
		t.Errorf("expected '1 tasks' in summary, got: %s", stdout)
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

	// In verbose mode, we should see the task output
	if !strings.Contains(stdout, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in verbose output, got: %s", stdout)
	}
}

// =============================================================================
// Run Command Tests - Dependencies
// =============================================================================

func TestRun_LinearDependencyChain(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "run", "step-c", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should run all three steps
	if !strings.Contains(stdout, "step-a") {
		t.Errorf("expected 'step-a' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "step-b") {
		t.Errorf("expected 'step-b' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "step-c") {
		t.Errorf("expected 'step-c' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "3 tasks") {
		t.Errorf("expected '3 tasks' in summary, got: %s", stdout)
	}

	// Verify order: step-a should appear before step-b, which should appear before step-c
	aPos := strings.Index(stdout, "step-a")
	bPos := strings.Index(stdout, "step-b")
	cPos := strings.Index(stdout, "step-c")

	if aPos > bPos || bPos > cPos {
		t.Errorf("tasks not in correct order: a=%d, b=%d, c=%d", aPos, bPos, cPos)
	}
}

func TestRun_DiamondDependency(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "run", "top", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should run all four tasks
	for _, task := range []string{"base", "left", "right", "top"} {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' in output, got: %s", task, stdout)
		}
	}
	if !strings.Contains(stdout, "4 tasks") {
		t.Errorf("expected '4 tasks' in summary, got: %s", stdout)
	}

	// Verify base runs before left/right, and left/right run before top
	basePos := strings.Index(stdout, "base")
	leftPos := strings.Index(stdout, "left")
	rightPos := strings.Index(stdout, "right")
	topPos := strings.Index(stdout, "top")

	if basePos > leftPos || basePos > rightPos {
		t.Errorf("base should run before left and right")
	}
	if leftPos > topPos || rightPos > topPos {
		t.Errorf("left and right should run before top")
	}
}

func TestRun_CompoundTask(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "deps")

	stdout, stderr, err := runNixTasks(t, binary, "run", "all", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should include tasks from both chains
	for _, task := range []string{"step-a", "step-b", "step-c", "base", "left", "right", "top", "all"} {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' in output, got: %s", task, stdout)
		}
	}
}

// =============================================================================
// Run Command Tests - Parallel Execution
// =============================================================================

func TestRun_ParallelExecution(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")

	// Run with 4 parallel jobs
	stdout, stderr, err := runNixTasks(t, binary, "run", "final", "-f", flakePath, "-j", "4")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// All tasks should complete
	for i := 1; i <= 4; i++ {
		taskName := "task-" + string(rune('0'+i))
		if !strings.Contains(stdout, taskName) {
			t.Errorf("expected '%s' in output, got: %s", taskName, stdout)
		}
	}
	if !strings.Contains(stdout, "final") {
		t.Errorf("expected 'final' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "5 tasks") {
		t.Errorf("expected '5 tasks' in summary, got: %s", stdout)
	}
}

func TestRun_SingleJob(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "parallel")

	// Run with only 1 parallel job (sequential)
	stdout, stderr, err := runNixTasks(t, binary, "run", "final", "-f", flakePath, "-j", "1")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// All tasks should still complete
	if !strings.Contains(stdout, "5 tasks") {
		t.Errorf("expected '5 tasks' in summary, got: %s", stdout)
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
	if err == nil {
		t.Fatal("expected error for failing task")
	}

	// Should show failure indicator
	if !strings.Contains(stdout, "✗") {
		t.Errorf("expected failure indicator in output, got: %s", stdout)
	}
	// Should show task name with failure
	if !strings.Contains(stdout, "fail") {
		t.Errorf("expected 'fail' task name in output, got: %s", stdout)
	}
}

func TestRun_ContinueOnError(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")

	// Task with continueOnError should allow dependent tasks to run
	stdout, stderr, err := runNixTasks(t, binary, "run", "after-continue", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Both tasks should run despite failure
	if !strings.Contains(stdout, "fail-continue") {
		t.Errorf("expected 'fail-continue' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "after-continue") {
		t.Errorf("expected 'after-continue' in output, got: %s", stdout)
	}
}

func TestRun_ContinueOnErrorFlag(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")

	// Run with --continue-on-error flag
	stdout, _, err := runNixTasks(t, binary, "run", "parallel-test", "-f", flakePath, "--continue-on-error")

	// Should still return error but run all independent tasks
	if err == nil {
		t.Log("Note: command succeeded despite expected failure")
	}

	// Independent tasks should still run
	if !strings.Contains(stdout, "independent-a") {
		t.Errorf("expected 'independent-a' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "independent-c") {
		t.Errorf("expected 'independent-c' in output, got: %s", stdout)
	}
}

func TestRun_FailFast(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "failing")

	// Default is fail-fast
	stdout, _, err := runNixTasks(t, binary, "run", "after-fail", "-f", flakePath)
	if err == nil {
		t.Fatal("expected error for task depending on failing task")
	}

	// The fail task should show failure
	if !strings.Contains(stdout, "fail") {
		t.Errorf("expected 'fail' in output, got: %s", stdout)
	}

	// Should show skipped in summary
	if strings.Contains(stdout, "after-fail") && strings.Contains(stdout, "✓") {
		t.Errorf("after-fail should not have succeeded")
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

	// Check task name
	if !strings.Contains(stdout, "Task: top") {
		t.Errorf("expected 'Task: top' in output, got: %s", stdout)
	}

	// Check dependencies
	if !strings.Contains(stdout, "Depends on:") {
		t.Errorf("expected 'Depends on:' section, got: %s", stdout)
	}
	if !strings.Contains(stdout, "task:left") {
		t.Errorf("expected 'task:left' in dependencies, got: %s", stdout)
	}
	if !strings.Contains(stdout, "task:right") {
		t.Errorf("expected 'task:right' in dependencies, got: %s", stdout)
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

	// Check that reverse dependencies are shown
	if !strings.Contains(stdout, "Depended on by:") {
		t.Errorf("expected 'Depended on by:' section, got: %s", stdout)
	}
	if !strings.Contains(stdout, "left") {
		t.Errorf("expected 'left' in dependents, got: %s", stdout)
	}
	if !strings.Contains(stdout, "right") {
		t.Errorf("expected 'right' in dependents, got: %s", stdout)
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

	// Should show statistics (case-insensitive check)
	if !strings.Contains(strings.ToLower(stdout), "tasks") {
		t.Errorf("expected task count in output, got: %s", stdout)
	}
}

// =============================================================================
// Shell Tests
// =============================================================================

func TestRun_TaskWithPackageDeps(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	// This task requires jq package
	stdout, stderr, err := runNixTasks(t, binary, "run", "check-jq", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should show jq version
	if !strings.Contains(stdout, "jq") {
		t.Errorf("expected jq version in output, got: %s", stdout)
	}
}

func TestShell_CommandFlag(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	// Run a command in the shell using -c flag
	stdout, stderr, err := runNixTasks(t, binary, "shell", "minimal", "-f", flakePath, "-c", "jq --version")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	// Should show jq version
	if !strings.Contains(stdout, "jq") {
		t.Errorf("expected jq version in output, got: %s", stdout)
	}
}

func TestShell_DefaultShell(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	// Run a command in the default shell (no shell name specified)
	stdout, stderr, err := runNixTasks(t, binary, "shell", "-f", flakePath, "-c", "echo $SHELL_TYPE")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	// Default shell extends extended which overrides SHELL_TYPE
	// The default shell doesn't set SHELL_TYPE, so it inherits from extended
	if !strings.Contains(stdout, "extended") {
		t.Errorf("expected SHELL_TYPE=extended in output, got: %s", stdout)
	}
}

func TestShell_InheritedPackages(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "shells")

	// The 'extended' shell should have both jq (from minimal) and curl
	stdout, stderr, err := runNixTasks(t, binary, "shell", "extended", "-f", flakePath, "-c", "jq --version && curl --version | head -1")
	if err != nil {
		t.Fatalf("nix-tasks shell failed: %v\nstderr: %s", err, stderr)
	}

	// Should have both tools
	if !strings.Contains(stdout, "jq") {
		t.Errorf("expected jq in inherited shell, got: %s", stdout)
	}
	if !strings.Contains(stdout, "curl") {
		t.Errorf("expected curl in extended shell, got: %s", stdout)
	}
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

	stdout, stderr, err := runNixTasks(t, binary, "run", "hello", "-f", flakePath)
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should show duration in parentheses
	if !strings.Contains(stdout, "(") || !strings.Contains(stdout, ")") {
		t.Errorf("expected duration in parentheses, got: %s", stdout)
	}

	// Duration should be in ms or s format
	if !strings.Contains(stdout, "ms)") && !strings.Contains(stdout, "s)") {
		t.Errorf("expected duration format (ms or s), got: %s", stdout)
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

	// With --stream, output should include the task output with prefixes
	if !strings.Contains(stdout, "Line 1") {
		t.Errorf("expected streaming output with 'Line 1', got: %s", stdout)
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
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")

	stdout, stderr, err := runNixTasks(t, binary, "run", "build", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run build failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	// Task should complete successfully
	if !strings.Contains(stdout, "✓") {
		t.Errorf("expected success checkmark in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "build") {
		t.Errorf("expected 'build' task in output, got: %s", stdout)
	}
}

func TestMkGoTask_BuildsBinaryToCorrectLocation(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "gotask")

	stdout, stderr, err := runNixTasks(t, binary, "run", "test", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run test failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	// Test task should run successfully (which verifies binary exists and runs)
	if !strings.Contains(stdout, "✓") {
		t.Errorf("expected success checkmark in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "test") {
		t.Errorf("expected 'test' task in output, got: %s", stdout)
	}

	// Should see the output from the Go program
	if !strings.Contains(stdout, "Hello from mkGoTask!") {
		t.Errorf("expected 'Hello from mkGoTask!' in output, got: %s", stdout)
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

	// Should run all tasks including the prepended one
	for _, task := range []string{"helm-set-app-version", "ko-publish", "helm-publish", "publish"} {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' in output, got: %s", task, stdout)
		}
	}

	// helm-set-app-version should run before ko-publish (prepended dependency)
	setPos := strings.Index(stdout, "helm-set-app-version")
	koPos := strings.Index(stdout, "ko-publish")
	if setPos > koPos {
		t.Errorf("helm-set-app-version should run before ko-publish")
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

	// Should run all tasks including the appended one
	for _, task := range []string{"ko-publish", "helm-publish", "cleanup", "publish-with-cleanup"} {
		if !strings.Contains(stdout, task) {
			t.Errorf("expected '%s' in output, got: %s", task, stdout)
		}
	}
}

func TestRun_PipedModifications(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	flakePath := filepath.Join(testdataDir(t), "modifiers")

	stdout, stderr, err := runNixTasks(t, binary, "run", "build", "-f", flakePath, "-v")
	if err != nil {
		t.Fatalf("nix-tasks run failed: %v\nstderr: %s", err, stderr)
	}

	// Should include prepended and appended commands in output
	if !strings.Contains(stdout, "Pre-build step") {
		t.Errorf("expected 'Pre-build step' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Post-build step") {
		t.Errorf("expected 'Post-build step' in output, got: %s", stdout)
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

	// Should show all three task dependencies
	if !strings.Contains(stdout, "task:helm-set-app-version") {
		t.Errorf("expected 'task:helm-set-app-version' in dependencies, got: %s", stdout)
	}
	if !strings.Contains(stdout, "task:ko-publish") {
		t.Errorf("expected 'task:ko-publish' in dependencies, got: %s", stdout)
	}
	if !strings.Contains(stdout, "task:helm-publish") {
		t.Errorf("expected 'task:helm-publish' in dependencies, got: %s", stdout)
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
