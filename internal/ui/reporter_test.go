package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func exitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}
	return err
}

func write(t *testing.T, r runner.Reporter, name, stdout, stderr string) {
	t.Helper()
	o, e := r.TaskOutput(name)
	_, _ = io.WriteString(o, stdout)
	_, _ = io.WriteString(e, stderr)
}

// testGraph builds a task graph from each task's dependencies.
func testGraph(t *testing.T, deps map[string][]string) *runner.TaskGraph {
	t.Helper()
	tasks := map[string]config.Task{}
	for name, d := range deps {
		tasks[name] = config.Task{Depends: d}
	}
	g, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// scenarioGraph is deploy needing build and test, with build needing gen.
func scenarioGraph(t *testing.T) *runner.TaskGraph {
	return testGraph(t, map[string][]string{
		"deploy": {"build", "test"},
		"build":  {"gen"},
		"test":   nil,
		"gen":    nil,
	})
}

// scenario drives a reporter through a run with a passing task, a cache hit,
// a failing task and a skipped dependent, and returns the results.
func scenario(t *testing.T, r runner.Reporter) []runner.TaskResult {
	t.Helper()
	build := runner.TaskResult{Name: "build", State: runner.Succeeded, Start: t0, Duration: 2300 * time.Millisecond}
	gen := runner.TaskResult{Name: "gen", State: runner.Succeeded, Cached: true}
	test := runner.TaskResult{Name: "test", State: runner.Failed, Error: exitError(t, 2), Start: t0, Duration: 150 * time.Millisecond}
	deploy := runner.TaskResult{Name: "deploy", State: runner.Skipped}

	r.TaskFinished(gen)
	r.TaskStarted("build")
	r.TaskStarted("test")
	write(t, r, "build", "compiling", "")
	write(t, r, "test", "running\n", "FAIL: thing\n")
	write(t, r, "build", "... ok\n", "")
	r.TaskFinished(test)
	r.TaskFinished(build)
	r.TaskFinished(deploy)
	return []runner.TaskResult{gen, build, test, deploy}
}

func TestLineReporter_Plain(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, false)
	results := scenario(t, r)
	r.Summary(scenarioGraph(t), []string{"deploy"}, results)

	want := `✓ gen (cached)
▶ build
▶ test
✗ test (150ms) (exit code 2)
✓ build (2.3s)
- deploy (skipped)

deploy - (skipped)
├─ build ✓ (2.3s)
│  └─ gen ✓ (cached)
└─ test ✗ (150ms) (exit code 2)

✗ test output:
    running
    FAIL: thing

Completed: 4 tasks (1 passed, 1 cached, 1 failed, 1 skipped)
`
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLineReporter_Stream(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, true)
	results := scenario(t, r)
	r.Summary(scenarioGraph(t), []string{"deploy"}, results)

	// Output already streamed is not repeated in the summary.
	want := `✓ gen (cached)
▶ build
▶ test
[test] running
[test] FAIL: thing
[build] compiling... ok
✗ test (150ms) (exit code 2)
✓ build (2.3s)
- deploy (skipped)

deploy - (skipped)
├─ build ✓ (2.3s)
│  └─ gen ✓ (cached)
└─ test ✗ (150ms) (exit code 2)

Completed: 4 tasks (1 passed, 1 cached, 1 failed, 1 skipped)
`
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLineReporter_StreamFlushesPartialLineAtFinish(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, true)
	r.TaskStarted("a")
	write(t, r, "a", "no newline", "")
	r.TaskFinished(runner.TaskResult{Name: "a", Duration: time.Millisecond})
	want := "▶ a\n[a] no newline\n✓ a (1ms)\n"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLineReporter_ColourFollowsSetting(t *testing.T) {
	defer SetColor(ColorEnabled())
	var out bytes.Buffer
	SetColor(true)
	NewLineReporter(&out, false).TaskFinished(runner.TaskResult{Name: "a", Duration: time.Millisecond})
	if !strings.Contains(out.String(), "\x1b[32m✓") {
		t.Errorf("expected colour, got %q", out.String())
	}
	out.Reset()
	SetColor(false)
	NewLineReporter(&out, false).TaskFinished(runner.TaskResult{Name: "a", Duration: time.Millisecond})
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("expected no escape sequences, got %q", out.String())
	}
}

func TestSummary_NonExitErrorShown(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, false)
	res := runner.TaskResult{Name: "img", State: runner.Failed, Error: errors.New("nix build failed: boom"), Duration: time.Second}
	r.TaskFinished(res)
	r.Summary(testGraph(t, map[string][]string{"img": nil}), []string{"img"}, []runner.TaskResult{res})
	if !strings.Contains(out.String(), "img output:\n    error: nix build failed: boom\n") {
		t.Errorf("expected error in summary, got:\n%s", out.String())
	}
}

func TestSummary_WarningsShown(t *testing.T) {
	SetColor(false)
	var out bytes.Buffer
	r := NewLineReporter(&out, false)
	res := runner.TaskResult{Name: "gen", State: runner.Succeeded, Warnings: []string{"not cached: inputs changed while it ran: a.go"}}
	r.TaskFinished(res)
	r.Summary(testGraph(t, map[string][]string{"gen": nil}), []string{"gen"}, []runner.TaskResult{res})
	want := "✓ gen (0ms)\n\ngen ✓ (0ms)\n\nwarning: gen: not cached: inputs changed while it ran: a.go\n\nCompleted: 1 tasks (1 passed)\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	var stderr bytes.Buffer
	raw := NewRawReporter("gen", io.Discard, &stderr)
	raw.TaskFinished(res)
	if stderr.String() != "warning: gen: not cached: inputs changed while it ran: a.go\n" {
		t.Errorf("raw stderr = %q", stderr.String())
	}
}

func TestWriteFailure_LogOnlyWhenTruncated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	res := runner.TaskResult{Name: "t", State: runner.Failed}

	var short strings.Builder
	writeFailure(&short, res, []byte(strings.Repeat("line\n", FailedOutputTailLines)))
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("output that fits the tail should not be logged, found %v", entries)
	}
	if strings.Contains(short.String(), "full output") || strings.Contains(short.String(), "omitted") {
		t.Errorf("unexpected truncation notice:\n%s", short.String())
	}

	var long strings.Builder
	var full strings.Builder
	for i := range FailedOutputTailLines + 10 {
		fmt.Fprintf(&full, "line %d\n", i)
	}
	writeFailure(&long, res, []byte(full.String()))
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 1 {
		t.Fatalf("expected one log file, found %v", entries)
	}
	path := tmp + "/" + entries[0].Name()
	got := long.String()
	if !strings.HasPrefix(got, "    ... 10 lines omitted ...\n    line 10\n") {
		t.Errorf("unexpected tail:\n%s", got)
	}
	if !strings.HasSuffix(got, "    full output: "+path+"\n") {
		t.Errorf("log path not printed:\n%s", got)
	}
	if data, _ := os.ReadFile(path); string(data) != full.String() {
		t.Error("log does not hold the full output")
	}
}

func TestWriteFailure_AllowedFailureNeverLogged(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	res := runner.TaskResult{Name: "t", State: runner.FailedAllowed}

	var out strings.Builder
	writeFailure(&out, res, []byte(strings.Repeat("line\n", FailedOutputTailLines+10)))
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("an allowed failure should not write a log, found %v", entries)
	}
	if !strings.Contains(out.String(), "... 10 lines omitted ...") || strings.Contains(out.String(), "full output") {
		t.Errorf("expected a truncated tail without a log path:\n%s", out.String())
	}
}

func TestRawReporter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewRawReporter("app", &stdout, &stderr)

	write(t, r, "dep-ok", "quiet\n", "")
	r.TaskFinished(runner.TaskResult{Name: "dep-ok", State: runner.Succeeded})
	write(t, r, "app", "to stdout\n", "to stderr\n")
	r.TaskFinished(runner.TaskResult{Name: "app", State: runner.Failed, Error: exitError(t, 1)})
	results := []runner.TaskResult{
		{Name: "dep-ok", State: runner.Succeeded, Duration: time.Second},
		{Name: "app", State: runner.Failed, Error: exitError(t, 1), Duration: time.Second},
	}
	r.Summary(testGraph(t, map[string][]string{"app": {"dep-ok"}, "dep-ok": nil}), []string{"app"}, results)
	if got := stderr.String(); got != "to stderr\n" {
		t.Errorf("a failing target should add nothing to its own output, stderr = %q", got)
	}

	stderr.Reset()
	r = NewRawReporter("app", &stdout, &stderr)
	write(t, r, "dep-bad", "why it broke\n", "")
	bad := runner.TaskResult{Name: "dep-bad", State: runner.Failed, Error: exitError(t, 3), Duration: time.Second}
	r.TaskFinished(bad)
	app := runner.TaskResult{Name: "app", State: runner.Skipped}
	r.TaskFinished(app)
	r.Summary(testGraph(t, map[string][]string{"app": {"dep-bad"}, "dep-bad": nil}), []string{"app"}, []runner.TaskResult{bad, app})

	if got := stdout.String(); got != "to stdout\n" {
		t.Errorf("stdout = %q, want only the target's output", got)
	}
	want := `
app - (skipped)
└─ dep-bad ✗ (1.0s) (exit code 3)

✗ dep-bad output:
    why it broke

Completed: 2 tasks (1 failed, 1 skipped)
`
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestResultTree(t *testing.T) {
	tasks := map[string]config.Task{
		"all":    {Depends: []string{"left", "right"}},
		"left":   {Depends: []string{"base"}},
		"right":  {Depends: []string{"base"}},
		"base":   {},
		"notify": {After: []string{"left"}},
		"other":  {Depends: []string{"base"}},
	}
	graph, err := runner.NewTaskGraph(tasks)
	if err != nil {
		t.Fatal(err)
	}
	ok := func(name string) runner.TaskResult {
		return runner.TaskResult{Name: name, State: runner.Succeeded, Duration: time.Second}
	}
	results := []runner.TaskResult{
		ok("base"), ok("left"), ok("right"), ok("all"), ok("other"),
		{Name: "notify", State: runner.FailedAllowed, Error: exitError(t, 1), Duration: time.Second},
	}

	got := strings.Join(resultTree(graph, []string{"all", "other"}, results), "\n")
	// A shared dependency is drawn once; later appearances are marked with *.
	want := `all ✓ (1.0s)
├─ left ✓ (1.0s)
│  ├─ base ✓ (1.0s)
│  └─ notify (after) ! (1.0s) (failure allowed)
└─ right ✓ (1.0s)
   └─ base ✓ (1.0s) *
other ✓ (1.0s)
└─ base ✓ (1.0s) *`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func newTestProgress(out io.Writer, width, height int, now *time.Time) *ProgressReporter {
	return NewProgressReporter(ProgressOptions{
		Out:       out,
		TailLines: 2,
		Size:      func() (int, int) { return width, height },
		Now:       func() time.Time { return *now },
	})
}

func TestProgressReporter(t *testing.T) {
	var out bytes.Buffer
	now := t0
	p := newTestProgress(&out, 40, 24, &now)

	p.TaskStarted("build")
	write(t, p, "build", "step 1\nstep 2\nstep 3\n", "")
	now = now.Add(1500 * time.Millisecond)
	p.render()
	frame1 := out.String()
	out.Reset()

	p.TaskFinished(runner.TaskResult{Name: "build", Duration: 1500 * time.Millisecond})
	frame2 := out.String()
	out.Reset()

	p.Summary(testGraph(t, map[string][]string{"build": nil}), []string{"build"}, []runner.TaskResult{{Name: "build", Duration: 1500 * time.Millisecond}})

	wantFrame1 := "⠋ build (1.5s)\n    step 2\n    step 3\n"
	if frame1 != wantFrame1 {
		t.Errorf("frame 1 = %q, want %q", frame1, wantFrame1)
	}
	// Moves up over the 3-row region, clears it, and prints the finished
	// line in its place.
	wantFrame2 := "\x1b[3A\r\x1b[J✓ build (1.5s)\n"
	if frame2 != wantFrame2 {
		t.Errorf("frame 2 = %q, want %q", frame2, wantFrame2)
	}
	if got := out.String(); got != "\nbuild ✓ (1.5s)\n\nCompleted: 1 tasks (1 passed)\n" {
		t.Errorf("summary = %q", got)
	}
}

func TestProgressReporter_FinishedTasksPrintedOnce(t *testing.T) {
	var out bytes.Buffer
	now := t0
	p := newTestProgress(&out, 40, 24, &now)
	p.TaskStarted("a")
	p.TaskStarted("b")
	p.TaskFinished(runner.TaskResult{Name: "a", Duration: time.Second})
	for range 5 {
		p.render()
	}
	p.TaskFinished(runner.TaskResult{Name: "b", Duration: time.Second})
	p.Stop()
	if n := strings.Count(out.String(), "✓ a"); n != 1 {
		t.Errorf("finished task printed %d times:\n%q", n, out.String())
	}
}

func TestProgressReporter_RegionFitsTerminal(t *testing.T) {
	var out bytes.Buffer
	now := t0
	const width, height = 20, 6
	p := newTestProgress(&out, width, height, &now)
	for i := range 10 {
		name := fmt.Sprintf("task-with-a-long-name-%d", i)
		p.TaskStarted(name)
		write(t, p, name, strings.Repeat("界", 50)+"\n", "")
	}
	p.mu.Lock()
	rows := p.regionRows()
	p.mu.Unlock()

	if len(rows) > height-1 {
		t.Errorf("region has %d rows, terminal fits %d", len(rows), height-1)
	}
	for _, row := range rows {
		if w := ansi.StringWidth(row); w >= width {
			t.Errorf("row %q is %d columns wide, terminal is %d", row, w, width)
		}
	}
	if last := rows[len(rows)-1]; !strings.Contains(last, "more running") {
		t.Errorf("expected hidden tasks to be counted, last row %q", last)
	}
}

func TestProgressReporter_TailsShrinkToFit(t *testing.T) {
	var out bytes.Buffer
	now := t0
	p := newTestProgress(&out, 40, 5, &now)
	p.TaskStarted("a")
	p.TaskStarted("b")
	write(t, p, "a", "1\n2\n3\n", "")
	write(t, p, "b", "1\n2\n3\n", "")
	p.mu.Lock()
	rows := p.regionRows()
	p.mu.Unlock()
	want := []string{"⠋ a (0ms)", "    3", "⠋ b (0ms)", "    3"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

func TestProgressReporter_InterruptRestoresCursor(t *testing.T) {
	var out bytes.Buffer
	now := t0
	p := newTestProgress(&out, 40, 24, &now)
	p.Start()
	p.Interrupt()
	if !strings.HasSuffix(out.String(), showCursor) {
		t.Errorf("Interrupt did not show the cursor: %q", out.String())
	}
	p.Stop()

	out.Reset()
	p = newTestProgress(&out, 40, 24, &now)
	p.Interrupt()
	p.Start()
	p.Stop()
	if strings.Contains(out.String(), hideCursor) {
		t.Errorf("cursor hidden after an interrupt: %q", out.String())
	}
}
