//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// jsonRun is the --output=json document, decoded independently of the
// CLI's own types so these tests pin the published schema.
type jsonRun struct {
	Version    int      `json:"version"`
	Targets    []string `json:"targets"`
	Flake      string   `json:"flake"`
	System     string   `json:"system"`
	StartedAt  string   `json:"startedAt"`
	FinishedAt string   `json:"finishedAt"`
	DurationMs int64    `json:"durationMs"`
	ExitCode   int      `json:"exitCode"`
	Error      *string  `json:"error"`
	Counts     struct {
		Passed          int `json:"passed"`
		Failed          int `json:"failed"`
		AllowedFailures int `json:"allowedFailures"`
		Cached          int `json:"cached"`
		Skipped         int `json:"skipped"`
		Interrupted     int `json:"interrupted"`
	} `json:"counts"`
	Warnings []string   `json:"warnings"`
	Tasks    []jsonTask `json:"tasks"`
}

type jsonTask struct {
	Name       string  `json:"name"`
	Label      string  `json:"label"`
	State      string  `json:"state"`
	Cached     bool    `json:"cached"`
	AfterHook  bool    `json:"afterHook"`
	ExitCode   *int    `json:"exitCode"`
	Error      *string `json:"error"`
	SkipReason *string `json:"skipReason"`
	StartedAt  *string `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
	DurationMs *int64  `json:"durationMs"`
	Log        *string `json:"log"`
}

// runResult is one nix-tasks run with --output=json.
type runResult struct {
	Doc    jsonRun
	Stderr string
	Code   int
	// State is the run's XDG_STATE_HOME, which holds its task logs.
	State string
}

// runJSON runs nix-tasks with --output=json from workDir, with cacheDir as
// its cache and a fresh state directory, and decodes the document. It fails
// the test unless stdout holds exactly one JSON document.
func runJSON(t *testing.T, workDir, cacheDir string, args ...string) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	state := t.TempDir()
	cmd := exec.CommandContext(ctx, nixTasksBinary(t), append([]string{"--output=json"}, args...)...)
	cmd.Dir = workDir
	cmd.Env = append(testEnviron(), "NIX_TASKS_CACHE_DIR="+cacheDir, "NIX_TASKS_TEST_LOG_DIR="+workDir, "XDG_STATE_HOME="+state)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("nix-tasks did not exit normally: %v\nstderr: %s", err, stderr.String())
		}
		code = exitErr.ExitCode()
	}

	res := runResult{Stderr: stderr.String(), Code: code, State: state}
	dec := json.NewDecoder(&stdout)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res.Doc); err != nil {
		t.Fatalf("stdout is not the JSON document: %v\nstdout: %s\nstderr: %s", err, stdout.String(), res.Stderr)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout holds more than one JSON document\nstderr: %s", res.Stderr)
	}
	if res.Doc.ExitCode != code {
		t.Errorf("document exitCode %d, process exited %d", res.Doc.ExitCode, code)
	}
	return res
}

// runFlakeJSON runs with --output=json on flake from a fresh working
// directory, with cacheDir as the cache.
func runFlakeJSON(t *testing.T, flake, cacheDir string, args ...string) runResult {
	t.Helper()
	return runJSON(t, t.TempDir(), cacheDir, append(args, "-f", flake)...)
}

// decodeJSON decodes a document printed by a run started some other way.
func decodeJSON(t *testing.T, stdout []byte) jsonRun {
	t.Helper()
	var doc jsonRun
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("stdout is not the JSON document: %v\n%s", err, stdout)
	}
	return doc
}

// log returns the contents of the named task's log.
func (r runResult) log(t *testing.T, name string) string {
	t.Helper()
	task := r.task(t, name)
	if task.Log == nil {
		t.Fatalf("task %s has no log", name)
	}
	data, err := os.ReadFile(*task.Log)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// task returns the named task's entry, failing the test if it is missing.
func (r runResult) task(t *testing.T, name string) jsonTask {
	t.Helper()
	for _, task := range r.Doc.Tasks {
		if task.Name == name {
			return task
		}
	}
	t.Fatalf("task %s not in the run: %+v", name, r.Doc.Tasks)
	return jsonTask{}
}

// names lists the run's tasks in document order.
func (r runResult) names() []string {
	var names []string
	for _, task := range r.Doc.Tasks {
		names = append(names, task.Name)
	}
	return names
}

// assertStates checks that exactly the tasks in want ran, with those states
// ("succeeded", "failed", "failed_allowed", "skipped" or "interrupted").
func (r runResult) assertStates(t *testing.T, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, task := range r.Doc.Tasks {
		got[task.Name] = task.State
	}
	if len(got) != len(want) {
		t.Errorf("ran %v, want %v\nstderr: %s", got, want, r.Stderr)
		return
	}
	for name, state := range want {
		if got[name] != state {
			t.Errorf("task %s: state %q, want %q\nstderr: %s", name, got[name], state, r.Stderr)
		}
	}
}

// assertCode checks the process exit code, which runJSON has already
// matched against the document's.
func (r runResult) assertCode(t *testing.T, want int) {
	t.Helper()
	if r.Code != want {
		t.Errorf("exit code %d, want %d\nstderr: %s", r.Code, want, r.Stderr)
	}
}

// finishedBefore reports whether first finished no later than second
// started; timestamps share one width, so they compare as strings.
func (r runResult) finishedBefore(t *testing.T, first, second string) bool {
	t.Helper()
	a, b := r.task(t, first), r.task(t, second)
	if a.FinishedAt == nil || b.StartedAt == nil {
		t.Fatalf("%s or %s has no timestamps", first, second)
	}
	return *a.FinishedAt <= *b.StartedAt
}

const jsonTasksConfig = `{
  tasks = {
    hello = { commands = [ "echo hello-out" "echo hello-err >&2" ]; cache = { inputs = [ "flake.nix" ]; outputs = [ ]; }; };
    fails = { commands = [ "echo about-to-fail" "exit 3" ]; };
    allowed = { commands = [ "exit 4" ]; continueOnError = true; };
    needs-fail = { depends = [ "fails" ]; commands = [ "echo unreachable" ]; };
    needs-allowed = { depends = [ "allowed" ]; commands = [ "echo after-allowed" ]; };
    hook = { after = [ "hello" ]; commands = [ "echo hook" ]; };
    group = tasksLib.mkCompoundTask { tasks = [ "hello" "needs-allowed" ]; };
  };
}`

func TestJSONOutput_Document(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	r := runJSON(t, dir, t.TempDir(), "run", "group")
	r.assertCode(t, 0)
	if r.Doc.Version != 1 || !slices.Equal(r.Doc.Targets, []string{"group"}) || r.Doc.Flake != dir || r.Doc.System != nixSystem() {
		t.Errorf("unexpected header: %+v", r.Doc)
	}
	if r.Doc.Error != nil || len(r.Doc.Warnings) != 0 {
		t.Errorf("unexpected error or warnings: %v %v", r.Doc.Error, r.Doc.Warnings)
	}
	if r.Doc.StartedAt > r.Doc.FinishedAt || r.Doc.DurationMs <= 0 {
		t.Errorf("run times out of order: %s .. %s (%dms)", r.Doc.StartedAt, r.Doc.FinishedAt, r.Doc.DurationMs)
	}
	r.assertStates(t, map[string]string{
		"hello": "succeeded", "hook": "succeeded", "allowed": "failed_allowed",
		"needs-allowed": "succeeded", "group": "succeeded",
	})
	if c := r.Doc.Counts; c.Passed != 4 || c.AllowedFailures != 1 || c.Failed+c.Cached+c.Skipped+c.Interrupted != 0 {
		t.Errorf("unexpected counts %+v", c)
	}
	// Dependencies first, ties broken by name.
	if got := r.names(); !slices.Equal(got, []string{"allowed", "hello", "hook", "needs-allowed", "group"}) {
		t.Errorf("tasks in order %v", got)
	}

	allowed := r.task(t, "allowed")
	if allowed.ExitCode == nil || *allowed.ExitCode != 4 || allowed.Error == nil {
		t.Errorf("allowed failure should carry its exit code and error: %+v", allowed)
	}
	hook := r.task(t, "hook")
	if !hook.AfterHook {
		t.Error("hook runs only because it follows hello, so it is an after-hook")
	}
	for _, name := range []string{"allowed", "hello", "needs-allowed", "group"} {
		if r.task(t, name).AfterHook {
			t.Errorf("%s is a target or a dependency, not an after-hook", name)
		}
	}
	if !r.finishedBefore(t, "allowed", "needs-allowed") || !r.finishedBefore(t, "hello", "hook") {
		t.Error("tasks should start after what they wait for")
	}
}

func TestJSONOutput_LogsHoldTaskOutput(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	r := runJSON(t, dir, t.TempDir(), "run", "hello")
	hello := r.task(t, "hello")
	if hello.Log == nil {
		t.Fatalf("hello has no log: %+v", hello)
	}
	runs := filepath.Join(r.State, "nix-tasks", "runs")
	if !strings.HasPrefix(*hello.Log, runs+string(filepath.Separator)) || filepath.Base(*hello.Log) != "hello.log" {
		t.Errorf("log %s should be <run dir>/hello.log under %s", *hello.Log, runs)
	}
	data, err := os.ReadFile(*hello.Log)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello-out\nhello-err\n" {
		t.Errorf("log holds %q", data)
	}
	if strings.Contains(r.Stderr, "hello-out") {
		t.Errorf("task output should only be in the log, stderr: %s", r.Stderr)
	}
}

func TestJSONOutput_FailureExitCodeAndSkipReason(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	r := runJSON(t, dir, t.TempDir(), "run", "needs-fail")
	r.assertCode(t, 3)
	r.assertStates(t, map[string]string{"fails": "failed", "needs-fail": "skipped"})
	fails := r.task(t, "fails")
	if fails.ExitCode == nil || *fails.ExitCode != 3 || fails.Error == nil || fails.Log == nil {
		t.Errorf("failed task should have exit code 3, an error and a log: %+v", fails)
	}
	if data, err := os.ReadFile(*fails.Log); err != nil || string(data) != "about-to-fail\n" {
		t.Errorf("failed task's log holds %q (%v)", data, err)
	}
	skipped := r.task(t, "needs-fail")
	if skipped.SkipReason == nil || *skipped.SkipReason != "fails failed" {
		t.Errorf("skip reason %v", skipped.SkipReason)
	}
	if skipped.Log != nil || skipped.StartedAt != nil || skipped.DurationMs != nil || skipped.ExitCode != nil {
		t.Errorf("a skipped task has no log, times or exit code: %+v", skipped)
	}
	if r.Doc.Counts.Failed != 1 || r.Doc.Counts.Skipped != 1 {
		t.Errorf("counts %+v", r.Doc.Counts)
	}
}

func TestJSONOutput_CachedTaskHasNoLog(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})
	cache := t.TempDir()

	runJSON(t, dir, cache, "run", "hello")
	r := runJSON(t, t.TempDir(), cache, "-f", dir, "run", "hello")
	hello := r.task(t, "hello")
	if !hello.Cached || hello.State != "succeeded" || hello.Log != nil || hello.ExitCode != nil {
		t.Errorf("second run should be a cache hit without a log: %+v", hello)
	}
	if r.Doc.Counts.Cached != 1 || r.Doc.Counts.Passed != 1 {
		t.Errorf("counts %+v (hook passes, hello is cached)", r.Doc.Counts)
	}
}

func TestJSONOutput_ErrorBeforeTasksRun(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	r := runJSON(t, dir, t.TempDir(), "run", "no-such-task")
	r.assertCode(t, 1)
	if r.Doc.Error == nil || !strings.Contains(*r.Doc.Error, "task not found: no-such-task") {
		t.Errorf("document should carry the error, got %v", r.Doc.Error)
	}
	if len(r.Doc.Tasks) != 0 {
		t.Errorf("no tasks should be listed: %v", r.names())
	}
}

func TestJSONOutput_RejectsRaw(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, nixTasksBinary(t), dir, "--output=json", "run", "--raw", "hello")
	if code != 1 || stdout != "" {
		t.Errorf("exit code %d, stdout %q", code, stdout)
	}
	if !strings.Contains(stderr, "--output=json cannot be combined with --raw") {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

func TestJSONOutput_OtherModesKeepNoRunLogs(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, jsonTasksConfig, map[string]string{})

	for _, mode := range []string{"plain", "stream", "github"} {
		state := t.TempDir()
		cmd := exec.Command(nixTasksBinary(t), "--output="+mode, "run", "hello", "-f", dir)
		cmd.Dir = t.TempDir()
		cmd.Env = append(testEnviron(), "NIX_TASKS_CACHE_DIR="+t.TempDir(), "XDG_STATE_HOME="+state)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, out)
		}
		if _, err := os.Stat(filepath.Join(state, "nix-tasks")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--output=%s should write no run logs (stat: %v)", mode, err)
		}
	}
}
