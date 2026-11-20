package ui

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

var update = flag.Bool("update", false, "rewrite golden files")

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// readable shows SGR sequences as <params> so golden files can be reviewed.
func readable(s string) string {
	return sgrPattern.ReplaceAllString(s, "<$1>")
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs (run with -update to accept):\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestGitHub(t *testing.T, out *bytes.Buffer, clock *fakeClock, summary string) *GitHubReporter {
	t.Helper()
	return NewGitHubReporter(GitHubOptions{
		Out:         out,
		SummaryPath: summary,
		Now:         clock.Now,
		Palette:     DefaultPalette,
		Token:       func() string { return "TOKEN" },
		SpoolDir:    t.TempDir(),
	})
}

func withColor(t *testing.T, on bool) {
	t.Helper()
	prev := ColorEnabled()
	SetColor(on)
	t.Cleanup(func() { SetColor(prev) })
}

const crds = "generate-crossplane-provider-aws-ec2-managed-resources-crds"

// githubScenario drives a run shaped like the one in
// docs/github-actions-output.md, plus a task with an abbreviated label.
func githubScenario(t *testing.T, g *GitHubReporter, clock *fakeClock) []runner.TaskResult {
	t.Helper()
	at := func(d time.Duration) time.Time { return t0.Add(d) }

	g.ConfigLoaded(1900 * time.Millisecond)
	g.Warn("generate has cache enabled but always runs: it depends on tools, which is not cached")
	g.RunPlanned(runner.RunPlan{
		Targets: []string{"deploy"},
		Tasks:   []string{"generate", crds, "lint", "vet", "fmt-check", "build", "deploy"},
		Labels: map[string]string{
			"generate": "generate", crds: "g-c-p-a-e-m-resources-crds", "lint": "lint",
			"vet": "vet", "fmt-check": "fmt-check", "build": "build", "deploy": "deploy",
		},
		Jobs: 4,
	})
	g.SetupFinished([]runner.EnvSetup{
		{Tasks: []string{"build", "generate", "lint"}, Cached: true, Duration: 100 * time.Millisecond},
		{Tasks: []string{crds, "fmt-check"}, Duration: 2600 * time.Millisecond},
	}, 2900*time.Millisecond)

	vet := runner.TaskResult{Name: "vet", State: runner.Succeeded, Cached: true, Saved: 18200 * time.Millisecond}
	g.TaskFinished(vet)
	g.TaskStarted("generate")
	g.TaskStarted("lint")
	g.TaskStarted(crds)
	write(t, g, "generate", "controller-gen object:headerFile=hack/boilerplate.go.txt\n", "")
	write(t, g, "lint", "", "\x1b[1mapis/v1/types.go:42:2\x1b[0m: ineffectual assignment to err (ineffassign)\n::endgroup::\n")
	write(t, g, crds, "wrote ", "")
	write(t, g, "generate", "controller-gen crd paths=./apis/...\n", "")
	write(t, g, crds, "package/crds/ec2.yaml\n", "")
	lint := runner.TaskResult{Name: "lint", State: runner.Failed, Error: exitError(t, 1), Start: at(0), Duration: 3100 * time.Millisecond}
	g.TaskFinished(lint)
	gen := runner.TaskResult{Name: "generate", State: runner.Succeeded, Start: at(0), Duration: 12400 * time.Millisecond,
		Cache: &runner.CacheOutcome{Uncached: "depends on uncached tools"}}
	g.TaskFinished(gen)
	crdsResult := runner.TaskResult{Name: crds, State: runner.Succeeded, Start: at(0), Duration: 13 * time.Second}
	g.TaskFinished(crdsResult)
	g.TaskStarted("fmt-check")
	write(t, g, "fmt-check", "main.go\n", "")
	fmtCheck := runner.TaskResult{Name: "fmt-check", State: runner.FailedAllowed, Error: exitError(t, 1), Start: at(13 * time.Second), Duration: 400 * time.Millisecond}
	g.TaskFinished(fmtCheck)
	g.TaskStarted("build")
	write(t, g, "build", "go build ./...\n", "")
	build := runner.TaskResult{Name: "build", State: runner.Succeeded, Start: at(14 * time.Second), Duration: 41 * time.Second,
		Warnings: []string{"not cached: inputs changed while it ran: go.sum"}}
	g.TaskFinished(build)
	deploy := runner.TaskResult{Name: "deploy", State: runner.Skipped, SkipReason: "lint failed"}
	g.TaskFinished(deploy)

	clock.now = at(56200 * time.Millisecond)
	return []runner.TaskResult{gen, crdsResult, lint, vet, fmtCheck, build, deploy}
}

func TestGitHubReporter_Golden(t *testing.T) {
	withColor(t, true)
	var out bytes.Buffer
	clock := &fakeClock{now: t0}
	summary := filepath.Join(t.TempDir(), "summary.md")
	g := newTestGitHub(t, &out, clock, summary)
	results := githubScenario(t, g, clock)
	g.Summary(nil, nil, results)
	g.Stop()

	assertGolden(t, "github_run.golden", readable(out.String()))
	md, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "github_summary.golden.md", string(md))
}

func TestGitHubReporter_NoColor(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	clock := &fakeClock{now: t0}
	g := newTestGitHub(t, &out, clock, "")
	g.Summary(nil, nil, githubScenario(t, g, clock))
	for _, code := range []string{"\x1b[36m", "\x1b[1;31m", "\x1b[2;"} {
		if strings.Contains(out.String(), code) {
			t.Errorf("expected no colour from nix-tasks, found %q in:\n%q", code, out.String())
		}
	}
	// Live output passes the tool's own colour through; the replay in the
	// Results section strips it.
	if !strings.Contains(out.String(), "lint                       │ \x1b[1mapis/v1/types.go:42:2\x1b[0m: ineffectual") {
		t.Errorf("expected the live line unchanged:\n%q", out.String())
	}
	if !strings.Contains(out.String(), "::stop-commands::TOKEN\napis/v1/types.go:42:2: ineffectual") {
		t.Errorf("expected the replayed line without colour:\n%q", out.String())
	}
}

func TestGitHubReporter_SingleTaskHasNoPrefix(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	clock := &fakeClock{now: t0}
	g := newTestGitHub(t, &out, clock, "")
	g.RunPlanned(runner.RunPlan{Targets: []string{crds}, Tasks: []string{crds}, Labels: map[string]string{crds: "g-c-p-a-e-m-resources-crds"}, Jobs: 4})
	g.TaskStarted(crds)
	write(t, g, crds, "::warning::from the tool\n", "")
	g.TaskFinished(runner.TaskResult{Name: crds, Duration: time.Second})

	want := "nix-tasks: " + crds + " · 1 task · -j 4\n" +
		"▶ " + crds + "\n" +
		"::warning::from the tool\n" +
		"━━ ✓ " + crds + " · 1.0s ━━\n"
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestGitHubReporter_PaletteBySortedName(t *testing.T) {
	withColor(t, true)
	var out bytes.Buffer
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, "")
	var tasks []string
	for i := 9; i >= 0; i-- {
		tasks = append(tasks, fmt.Sprintf("t%d", i))
	}
	g.RunPlanned(runner.RunPlan{Targets: []string{"t9"}, Tasks: tasks, Jobs: 1})
	for i, want := range []string{"36", "35", "34", "96", "95", "94", "36", "35", "34", "96"} {
		name := fmt.Sprintf("t%d", i)
		o, _ := g.TaskOutput(name)
		_, _ = fmt.Fprintln(o, "x")
		if line := fmt.Sprintf("\x1b[%sm%s │\x1b[0m x\n", want, name); !strings.Contains(out.String(), line) {
			t.Errorf("%s: expected colour %s, output:\n%q", name, want, out.String())
		}
	}
}

func TestGitHubReporter_SetupGroup(t *testing.T) {
	withColor(t, false)

	t.Run("failure is wrapped in stop-commands", func(t *testing.T) {
		var out bytes.Buffer
		g := newTestGitHub(t, &out, &fakeClock{now: t0}, "")
		g.ConfigLoaded(time.Second)
		g.SetupFinished([]runner.EnvSetup{
			{Tasks: []string{"a", "b"}, Duration: 2 * time.Second},
			{Tasks: []string{"c"}, Err: errors.New("nix print-dev-env failed:\n::endgroup::"), Duration: 300 * time.Millisecond},
		}, 3*time.Second)
		want := "::group::Preparing 2 environments (4.0s)\n" +
			"evaluated nixTasksConfig (1.0s)\n" +
			"environment for a, b: built with nix print-dev-env (2.0s)\n" +
			"environment for c: failed (0.3s)\n" +
			"::stop-commands::TOKEN\n" +
			"nix print-dev-env failed:\n" +
			"::endgroup::\n" +
			"::TOKEN::\n" +
			"::endgroup::\n"
		if got := out.String(); got != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("no environments", func(t *testing.T) {
		var out bytes.Buffer
		g := newTestGitHub(t, &out, &fakeClock{now: t0}, "")
		g.ConfigLoaded(1500 * time.Millisecond)
		g.SetupFinished(nil, 0)
		want := "::group::Loading configuration (1.5s)\nevaluated nixTasksConfig (1.5s)\n::endgroup::\n"
		if got := out.String(); got != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})
}

func TestGitHubReporter_ResultsShowsNonExitError(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, "")
	res := runner.TaskResult{Name: "img", State: runner.Failed, Error: errors.New("nix build failed: boom"), Duration: time.Second}
	g.TaskFinished(res)
	g.Summary(nil, nil, []runner.TaskResult{res})
	for _, want := range []string{
		"::group::✗ img · 1.0s\n::stop-commands::TOKEN\nerror: nix build failed: boom\n::TOKEN::\n::endgroup::\n",
		"::error title=nix-tasks%3A img failed::img failed after 1.0s: nix build failed: boom. Expand \"✗ img\" in the Results section of the log.\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in:\n%s", want, out.String())
		}
	}
}

func TestGitHubReporter_StopRemovesSpool(t *testing.T) {
	var out bytes.Buffer
	g := NewGitHubReporter(GitHubOptions{Out: &out})
	o, _ := g.TaskOutput("a")
	_, _ = fmt.Fprintln(o, "x")
	dir := g.spoolDir
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected a spool dir: %v", err)
	}
	g.Stop()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("spool dir %s still exists after Stop", dir)
	}
}

func TestAnnotations_Limit(t *testing.T) {
	var results []runner.TaskResult
	for i := range 12 {
		results = append(results, runner.TaskResult{Name: fmt.Sprintf("f%02d", i), State: runner.Failed, Error: exitError(t, 2), Duration: time.Second})
	}
	for i := range 3 {
		results = append(results, runner.TaskResult{Name: fmt.Sprintf("w%d", i), State: runner.FailedAllowed, Error: exitError(t, 1), Duration: time.Second})
	}
	got := annotations(nil, results)
	if len(got) != 13 {
		t.Fatalf("expected 10 errors and 3 warnings, got %d:\n%s", len(got), strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(got[8], "::error title=nix-tasks%3A f08 failed::") {
		t.Errorf("9th annotation = %q", got[8])
	}
	if want := "::error title=nix-tasks%3A more failures::…and 3 more failed tasks; see the job summary"; got[9] != want {
		t.Errorf("10th annotation = %q, want %q", got[9], want)
	}
	if want := "::warning title=nix-tasks%3A w0 failed (allowed)::w0 exited with code 1 after 1.0s; continueOnError is set, so the run continued."; got[10] != want {
		t.Errorf("first warning = %q, want %q", got[10], want)
	}

	var warns []runner.TaskResult
	for i := range 11 {
		warns = append(warns, runner.TaskResult{Name: fmt.Sprintf("w%02d", i), State: runner.FailedAllowed, Error: exitError(t, 1)})
	}
	got = annotations(nil, warns)
	if want := "::warning title=nix-tasks%3A more warnings::…and 2 more warnings; see the job summary"; len(got) != 10 || got[9] != want {
		t.Errorf("got %d warnings, last %q", len(got), got[len(got)-1])
	}
}

func TestAnnotations_CacheWarningsComeFirst(t *testing.T) {
	results := []runner.TaskResult{
		{Name: "lint", State: runner.FailedAllowed, Error: exitError(t, 1), Duration: time.Second},
		{Name: "gen", State: runner.Succeeded, Warnings: []string{"not cached: inputs changed while it ran: a.go"}},
	}
	got := annotations([]string{"gen has cache enabled but always runs: it depends on tools, which is not cached"}, results)
	want := []string{
		"::warning title=nix-tasks::gen has cache enabled but always runs: it depends on tools, which is not cached",
		"::warning title=nix-tasks%3A gen::not cached: inputs changed while it ran: a.go",
	}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || !strings.Contains(got[2], "lint failed (allowed)") {
		t.Errorf("got:\n%s", strings.Join(got, "\n"))
	}
}

func TestWorkflowCommand_Escapes(t *testing.T) {
	got := workflowCommand("error", "a: b, c%", "line 1\nline 2\r 100%")
	want := "::error title=a%3A b%2C c%25::line 1%0Aline 2%0D 100%25"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJobSummary_DetailsShowLast50Lines(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	summary := filepath.Join(t.TempDir(), "summary.md")
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, summary)
	o, _ := g.TaskOutput("big")
	for i := range 80 {
		_, _ = fmt.Fprintf(o, "line %d ```\n", i)
	}
	res := runner.TaskResult{Name: "big", State: runner.Failed, Error: exitError(t, 1), Duration: time.Second}
	g.TaskFinished(res)
	g.Summary(nil, nil, []runner.TaskResult{res})

	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "<details><summary>✗ big, last 50 lines</summary>\n\n````text\nline 30 ```\n") {
		t.Errorf("expected the last 50 lines in a longer fence:\n%s", md)
	}
	if strings.Contains(md, "line 29 ") || !strings.Contains(md, "line 79 ```\n````\n\n</details>\n") {
		t.Errorf("unexpected details:\n%s", md)
	}
}

func TestJobSummary_AppendsToExistingSummary(t *testing.T) {
	withColor(t, false)
	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(summary, []byte("earlier step\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, summary)
	g.Summary(nil, nil, []runner.TaskResult{{Name: "a", Duration: time.Second}})
	data, _ := os.ReadFile(summary)
	if !strings.HasPrefix(string(data), "earlier step\n### nix-tasks: ") {
		t.Errorf("summary = %q", data)
	}
}

func TestFitSummary(t *testing.T) {
	table := "### t\n| a |\n| b |\n"
	details := []string{"\n<details>one</details>\n", "\n<details>two</details>\n"}

	if got := fitSummary(table, details, MaxJobSummaryBytes); got != table+details[0]+details[1] {
		t.Errorf("everything fits, got %q", got)
	}

	limit := len(table) + len(details[0]) + len(summaryTruncatedNote)
	if got := fitSummary(table, details, limit); got != table+details[0]+summaryTruncatedNote {
		t.Errorf("one block fits, got %q", got)
	}

	limit = len("### t\n| a |\n") + len(summaryTruncatedNote) + 2
	if got := fitSummary(table, details, limit); got != "### t\n| a |\n"+summaryTruncatedNote {
		t.Errorf("table cut at a line, got %q", got)
	}
}

func TestJobSummary_StaysUnderLimit(t *testing.T) {
	withColor(t, false)
	summary := filepath.Join(t.TempDir(), "summary.md")
	var out bytes.Buffer
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, summary)
	long := strings.Repeat("x", 4000) + "\n"
	var results []runner.TaskResult
	for i := range 20 {
		name := fmt.Sprintf("t%02d", i)
		o, _ := g.TaskOutput(name)
		for range 50 {
			_, _ = o.Write([]byte(long))
		}
		r := runner.TaskResult{Name: name, State: runner.Failed, Error: exitError(t, 1)}
		g.TaskFinished(r)
		results = append(results, r)
	}
	g.Summary(nil, nil, results)
	info, err := os.Stat(summary)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > MaxJobSummaryBytes {
		t.Errorf("summary is %d bytes, over the limit", info.Size())
	}
	data, _ := os.ReadFile(summary)
	if !strings.HasSuffix(string(data), summaryTruncatedNote) {
		t.Error("expected the truncation note")
	}
}

func TestFormatSeconds(t *testing.T) {
	for d, want := range map[time.Duration]string{
		400 * time.Millisecond:   "0.4s",
		12400 * time.Millisecond: "12.4s",
		125 * time.Second:        "2m05s",
	} {
		if got := formatSeconds(d, false); got != want {
			t.Errorf("formatSeconds(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestGitHubReporter_ParallelLinesStayWhole(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, "")
	g.RunPlanned(runner.RunPlan{Targets: []string{"a"}, Tasks: []string{"a", "bb"}, Jobs: 2})
	done := make(chan struct{})
	for _, name := range []string{"a", "bb"} {
		go func() {
			defer func() { done <- struct{}{} }()
			o, e := g.TaskOutput(name)
			for i := range 200 {
				_, _ = fmt.Fprintf(o, "%s out %d\n", name, i)
				_, _ = fmt.Fprintf(e, "%s err %d\n", name, i)
			}
		}()
	}
	<-done
	<-done
	g.Stop()
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n")[1:] {
		if !regexp.MustCompile(`^(a  │ a|bb │ bb) (out|err) \d+$`).MatchString(line) {
			t.Fatalf("mangled line %q", line)
		}
	}
}

func TestGitHubReporter_SeveralTargets(t *testing.T) {
	withColor(t, false)
	var out bytes.Buffer
	summary := filepath.Join(t.TempDir(), "summary.md")
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, summary)
	g.RunPlanned(runner.RunPlan{Targets: []string{"lint", "build"}, Tasks: []string{"build", "lint"}, Jobs: 4})
	g.Summary(nil, nil, []runner.TaskResult{{Name: "lint"}, {Name: "build"}})
	if !strings.HasPrefix(out.String(), "nix-tasks: lint build · 2 tasks · -j 4\n") {
		t.Errorf("header should name every target:\n%s", out.String())
	}
	md, _ := os.ReadFile(summary)
	if !strings.HasPrefix(string(md), "### nix-tasks: lint build ✓\n") {
		t.Errorf("summary title should name every target:\n%s", md)
	}
}

// commandRun drives a github run of tasks through steps and returns the log
// followed by the job summary, for the workflow command golden file.
func commandRun(t *testing.T, tasks []string, steps func(g *GitHubReporter) []runner.TaskResult) string {
	t.Helper()
	var out bytes.Buffer
	summary := filepath.Join(t.TempDir(), "summary.md")
	g := newTestGitHub(t, &out, &fakeClock{now: t0}, summary)
	g.RunPlanned(runner.RunPlan{Targets: tasks, Tasks: tasks, Jobs: 4})
	g.Summary(nil, nil, steps(g))
	g.Stop()
	md, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	return readable(out.String()) + "--- job summary\n" + string(md)
}

func passed(name string) runner.TaskResult {
	return runner.TaskResult{Name: name, Duration: time.Second}
}

func failedWith(t *testing.T, name string) runner.TaskResult {
	return runner.TaskResult{Name: name, State: runner.Failed, Error: exitError(t, 1), Duration: time.Second}
}

func TestGitHubReporter_WorkflowCommands(t *testing.T) {
	withColor(t, false)
	scenarios := []struct {
		name  string
		tasks []string
		steps func(g *GitHubReporter) []runner.TaskResult
	}{
		{"annotations pass through", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "::error file=apis/v1/types.go,line=42,col=2::ineffectual assignment to err\n"+
				"::warning::deprecated flag\n"+
				"  ::notice title=Coverage::81%\n"+
				"plain line\n", "")
			lint := failedWith(t, "lint")
			g.TaskFinished(lint)
			return []runner.TaskResult{lint, passed("build")}
		}},
		{"add-mask passes through and is redacted afterwards", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "token s3cr3t before it is masked\n::add-mask::s3cr3t\ntoken s3cr3t after it is masked\n", "")
			lint := failedWith(t, "lint")
			g.TaskFinished(lint)
			return []runner.TaskResult{lint, passed("build")}
		}},
		{"a tool group", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "::group::Lint details\nchecked 12 files\n::endgroup::\ndone\n", "")
			g.TaskFinished(passed("lint"))
			return []runner.TaskResult{passed("lint"), passed("build")}
		}},
		{"an unclosed group is closed when the task finishes", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "::group::Lint details\nchecked 12 files\n", "")
			g.TaskFinished(passed("lint"))
			return []runner.TaskResult{passed("lint"), passed("build")}
		}},
		{"an unresumed stop-commands is resumed when the task finishes", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "::stop-commands::tooltoken\n::endgroup::\n::error::not an annotation here\n", "")
			lint := failedWith(t, "lint")
			g.TaskFinished(lint)
			return []runner.TaskResult{lint, passed("build")}
		}},
		{"a stray endgroup is shown as text", []string{"lint", "build"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "::endgroup::\n  ::endgroup::\n", "")
			g.TaskFinished(passed("lint"))
			return []runner.TaskResult{passed("lint"), passed("build")}
		}},
		{"a stray endgroup in a run of one task", []string{"lint"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("lint")
			write(t, g, "lint", "plain line\n::endgroup::\n::warning::kept\n", "")
			g.TaskFinished(passed("lint"))
			return []runner.TaskResult{passed("lint")}
		}},
		{"interleaved tasks", []string{"gen", "lint"}, func(g *GitHubReporter) []runner.TaskResult {
			g.TaskStarted("gen")
			g.TaskStarted("lint")
			write(t, g, "gen", "::group::Generate\nwrote a.go\n", "")
			write(t, g, "lint", "::stop-commands::linttoken\n", "")
			write(t, g, "gen", "::warning::ignored by GitHub while lint has stopped commands\n", "")
			g.TaskFinished(passed("gen"))
			write(t, g, "lint", "::group::not a group while stopped\nlint line\n", "")
			lint := failedWith(t, "lint")
			g.TaskFinished(lint)
			return []runner.TaskResult{passed("gen"), lint}
		}},
	}
	var golden strings.Builder
	for _, sc := range scenarios {
		fmt.Fprintf(&golden, "=== %s\n%s\n", sc.name, commandRun(t, sc.tasks, sc.steps))
	}
	assertGolden(t, "github_workflow_commands.golden", golden.String())
}

func TestParseWorkflowCommand(t *testing.T) {
	tests := []struct {
		line, name, value string
		ok                bool
	}{
		{"::group::Build", "group", "Build", true},
		{"  ::endgroup::", "endgroup", "", true},
		{"::error file=a.go,line=1::msg", "error", "msg", true},
		{"::error title=a:b::msg", "error", "msg", true},
		{"::tok123::", "tok123", "", true},
		{"::no end", "", "", false},
		{"::::", "", "", false},
		{"x ::group::", "", "", false},
		{"plain", "", "", false},
	}
	for _, tt := range tests {
		name, value, ok := parseWorkflowCommand([]byte(tt.line))
		if name != tt.name || value != tt.value || ok != tt.ok {
			t.Errorf("parseWorkflowCommand(%q) = %q, %q, %v; want %q, %q, %v", tt.line, name, value, ok, tt.name, tt.value, tt.ok)
		}
	}
}
