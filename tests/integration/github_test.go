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
	"strings"
	"testing"
	"time"
)

const githubTasksConfig = `{
  tasks = {
    generate-crossplane-provider-aws-ec2-managed-resources-crds.commands = [ "echo 'wrote crds'" ];
    ok = { label = "fine"; commands = [ "echo 'ok line'" ]; };
    bad.commands = [ "echo 'bad output'" "echo '::error file=main.go,line=1::from the tool'" "echo '::add-mask::hunter2'" "echo 'pw hunter2'" "echo '::endgroup::'" "echo '::group::left open'" "exit 3" ];
    allowed = { continueOnError = true; commands = [ "echo 'allowed output'" "exit 1" ]; };
    after-bad = { depends = [ "task:bad" ]; commands = [ "true" ]; };
    all.depends = [
      "task:generate-crossplane-provider-aws-ec2-managed-resources-crds"
      "task:ok" "task:allowed" "task:after-bad"
    ];
  };
}`

// runUnderGitHub runs nix-tasks as GitHub Actions would, with a job summary
// file, and returns stdout, the summary and the exit code.
func runUnderGitHub(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	summary := filepath.Join(t.TempDir(), "summary.md")

	env := append(testEnviron("NO_COLOR", "CI"), "GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY="+summary, "NIX_TASKS_CACHE_DIR="+t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, nixTasksBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("nix-tasks did not run: %v\nstderr: %s", err, stderr.String())
	}

	md, err := os.ReadFile(summary)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return stdout.String(), string(md), code
}

func TestGitHubOutput(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, githubTasksConfig, map[string]string{})

	raw, summary, code := runUnderGitHub(t, dir, "run", "all", "--continue-on-error", "-j", "4")
	if code != 3 {
		t.Errorf("exit code = %d, want the failed task's 3\n%s", code, raw)
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Error("expected colour under GitHub Actions although stdout is a pipe")
	}
	out := stripANSI(raw)

	for _, want := range []string{
		"nix-tasks: all · 6 tasks · -j 4\n",
		"▶ g-c-p-a-e-m-resources-crds = generate-crossplane-provider-aws-ec2-managed-resources-crds\n",
		"g-c-p-a-e-m-resources-crds │ wrote crds\n",
		fmt.Sprintf("%-26s │ ok line\n", "fine"),
		fmt.Sprintf("%-26s │ bad output\n", "bad"),
		"\n::error file=main.go,line=1::from the tool\n",
		"\n::add-mask::hunter2\n",
		fmt.Sprintf("%-26s │ pw hunter2\n", "bad"),
		fmt.Sprintf("%-26s │ ::endgroup::\n", "bad"),
		"━━ ✗ bad failed · exit 3 · ",
		"━━ ! allowed failed, allowed · exit 1 · ",
		"━━ - after-bad skipped · bad failed ━━\n",
		"- after-bad · skipped (bad failed)\n",
		"::error title=nix-tasks%3A bad failed::bad exited with code 3 after ",
		"::warning title=nix-tasks%3A allowed failed (allowed)::allowed exited with code 1 after ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}

	setup := regexp.MustCompile(`(?m)^::group::Preparing 1 environment \(\d+\.\ds\)\nevaluated nixTasksConfig \(\d+\.\ds\)\nenvironment for after-bad, allowed, bad, generate-crossplane-provider-aws-ec2-managed-resources-crds, ok: (built with nix print-dev-env|cached environment) \(\d+\.\ds\)\n::endgroup::\n▶ `)
	if !setup.MatchString(out) {
		t.Errorf("expected the setup group before any task starts:\n%s", out)
	}

	group := regexp.MustCompile(`(?m)^::group::✗ bad · exit 3 · \d+\.\ds\n::stop-commands::([0-9a-f]{32})\nbad output\n::error file=main.go,line=1::from the tool\n::add-mask::\*\*\*\npw \*\*\*\n::endgroup::\n::group::left open\n::([0-9a-f]{32})::\n::endgroup::\n`)
	m := group.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("expected bad's output in a protected group:\n%s", out)
	}
	if m[1] != m[2] {
		t.Errorf("stop-commands token %s not closed by %s", m[1], m[2])
	}
	if strings.Contains(out, "::group::✓") {
		t.Errorf("passed tasks should have plain Results lines:\n%s", out)
	}

	results := out[strings.Index(out, " Results "):]
	var order []string
	for _, line := range strings.Split(results, "\n") {
		if f := regexp.MustCompile(`^(?:::group::)?[✓✗!\-●⏹] (\S+) · `).FindStringSubmatch(line); f != nil {
			order = append(order, f[1])
		}
	}
	wantOrder := "allowed bad after-bad generate-crossplane-provider-aws-ec2-managed-resources-crds ok all"
	if strings.Join(order, " ") != wantOrder {
		t.Errorf("Results order = %v, want %s", order, wantOrder)
	}

	live := out[:strings.Index(out, " Results ")]
	if !strings.Contains(live, "\n::group::left open\n") {
		t.Errorf("expected the tool's group unprefixed:\n%s", live)
	}
	// One closes the setup group, the other the group bad left open; bad's
	// own stray ::endgroup:: is shown prefixed.
	if n := len(regexp.MustCompile(`(?m)^::endgroup::$`).FindAllString(live, -1)); n != 2 {
		t.Errorf("expected 2 ::endgroup:: commands before the Results, got %d:\n%s", n, live)
	}

	if strings.Contains(summary, "hunter2") {
		t.Errorf("the job summary shows a value the task masked:\n%s", summary)
	}
	for _, want := range []string{
		"### nix-tasks: all ✗\n",
		"| bad | ✗ failed | ",
		" | exit 3 |\n",
		"| after-bad | - skipped | — | bad failed |\n",
		"| allowed | ! failed (allowed) | ",
		"**1 failed** · 1 allowed failure · 2 passed · 2 skipped · ",
		"<details><summary>✗ bad output</summary>\n\n```text\nbad output\n::error file=main.go,line=1::from the tool\n::add-mask::***\npw ***\n::endgroup::\n::group::left open\n```\n\n</details>\n",
		"<details><summary>! allowed output</summary>",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("expected %q in the job summary:\n%s", want, summary)
		}
	}
}

func TestGitHubOutput_ExplicitOutputWins(t *testing.T) {
	skipIfNoNix(t)
	dir := writeTempFlake(t, `{ tasks.hello.commands = [ "echo hi" ]; }`, map[string]string{})

	out, summary, code := runUnderGitHub(t, dir, "run", "hello", "--output=plain")
	if code != 0 {
		t.Fatalf("exit code %d\n%s", code, out)
	}
	if strings.Contains(out, "::") || summary != "" {
		t.Errorf("expected plain output and no job summary, got:\n%s\nsummary: %q", out, summary)
	}
	if !strings.Contains(stripANSI(out), "✓ hello") {
		t.Errorf("expected a plain result line:\n%s", out)
	}
}

func TestHelpers_IgnoreInheritedGitHubActions(t *testing.T) {
	skipIfNoNix(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "summary.md"))
	dir := writeTempFlake(t, `{ tasks.hello.commands = [ "echo hi" ]; }`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, nixTasksBinary(t), dir, "run", "hello", "-f", dir)
	if code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "::") {
		t.Errorf("helpers should not pass GitHub Actions variables on, got:\n%s", stdout)
	}
}

func TestLabels_ValidateWarnsAndDescribeShows(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, `{
  tasks.test-unit-crossplane-provider-aws-ec2-managed-resources.commands = [ "true" ];
  tasks.generate-crossplane-provider-aws-ec2-managed-resources-crds = { label = "ec2-crds"; commands = [ "true" ]; };
}`, map[string]string{})

	stdout, stderr, code := runNixTasksIn(t, binary, dir, "validate", "-f", dir)
	if code != 0 {
		t.Fatalf("validate failed (%d): %s", code, stderr)
	}
	out := stripANSI(stdout)
	if !strings.Contains(out, "warning: task 'test-unit-crossplane-provider-aws-ec2-managed-resources': name is longer than 24 characters, so streamed output abbreviates it to 't-u-c-p-a-e-managed-resources'") {
		t.Errorf("expected a warning for the unlabelled task:\n%s", out)
	}
	if strings.Contains(out, "generate-crossplane") {
		t.Errorf("a labelled task should not be warned about:\n%s", out)
	}

	for task, want := range map[string]string{
		"generate-crossplane-provider-aws-ec2-managed-resources-crds": "Label:\n  ec2-crds\n",
		"test-unit-crossplane-provider-aws-ec2-managed-resources":     "Label:\n  t-u-c-p-a-e-managed-resources (abbreviated; set `label` to choose one)\n",
	} {
		stdout, stderr, code := runNixTasksIn(t, binary, dir, "describe", task, "-f", dir)
		if code != 0 {
			t.Fatalf("describe %s failed (%d): %s", task, code, stderr)
		}
		if !strings.Contains(stripANSI(stdout), want) {
			t.Errorf("describe %s: expected %q in:\n%s", task, want, stdout)
		}
	}
}

func TestConfig_DuplicateLabelsAreAnError(t *testing.T) {
	skipIfNoNix(t)
	binary := nixTasksBinary(t)
	dir := writeTempFlake(t, `{
  tasks.a = { label = "x"; commands = [ "true" ]; };
  tasks.b = { label = "x"; commands = [ "true" ]; };
}`, map[string]string{})

	_, stderr, code := runNixTasksIn(t, binary, dir, "list", "-f", dir)
	if code == 0 {
		t.Fatal("expected duplicate labels to fail")
	}
	if !strings.Contains(stderr, "task 'b': label 'x' is already used by task 'a'") {
		t.Errorf("unexpected error: %s", stderr)
	}
}
