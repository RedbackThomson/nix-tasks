package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func plain(lines []string) string {
	return ansi.Strip(strings.Join(lines, "\n"))
}

func TestTaskDetail(t *testing.T) {
	task := config.Task{
		Description:     "Deploy the app",
		Deps:            []string{"kubectl", "helm"},
		Depends:         []string{"build", "test"},
		After:           []string{"release"},
		Script:          "set -e\nhelm upgrade app ./chart\n",
		Env:             map[string]string{"ZONE": "eu", "APP": "web"},
		WorkingDir:      "deploy",
		ContinueOnError: true,
	}
	got := plain(taskDetail("deploy", task, 60))
	for _, want := range []string{
		"deploy\nDeploy the app",
		"Type         shell",
		"Packages     kubectl, helm",
		"Depends on   build, test",
		"Runs after   release",
		"Working dir  deploy",
		"On failure   continue (continueOnError)",
		"Env\n  APP=web\n  ZONE=eu",
		"Script\n  set -e\n  helm upgrade app ./chart",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	got = plain(taskDetail("build", config.Task{Type: config.TaskTypeBuild, Commands: []string{"go build"}}, 60))
	if !strings.Contains(got, "Type         build") || !strings.Contains(got, "Commands\n  go build") {
		t.Errorf("unexpected detail:\n%s", got)
	}
	if strings.Contains(got, "Env") || strings.Contains(got, "Packages") {
		t.Errorf("expected empty fields to be left out:\n%s", got)
	}
}

func TestTaskDetailWrapsLongValues(t *testing.T) {
	var deps []string
	for i := range 20 {
		deps = append(deps, fmt.Sprintf("package-%d", i))
	}
	for _, line := range taskDetail("x", config.Task{Deps: deps}, 40) {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line is %d wide, want at most 40: %q", w, ansi.Strip(line))
		}
	}
}

func TestShellDetail(t *testing.T) {
	got := plain(shellDetail("default", testConfig().DevShells, 60))
	for _, want := range []string{
		"Extends      ci → minimal",
		"Packages     kubectl",
		"Inherits     go, bash",
		"Env\n  EDITOR=vim",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if got := plain(shellDetail("minimal", testConfig().DevShells, 60)); strings.Contains(got, "Extends") {
		t.Errorf("expected no extends chain for a root shell:\n%s", got)
	}
}

func TestDetailPaneBesideListOnWideTerminals(t *testing.T) {
	m := update(t, loaded(t, testConfig()), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = press(t, m, "end")
	v := view(m)
	if !strings.Contains(v, "│ test\n") {
		t.Errorf("expected the selected task's name at the top of the detail pane, got:\n%s", v)
	}
	if !strings.Contains(v, "Depends on   build") {
		t.Errorf("expected the selected task's detail, got:\n%s", v)
	}
}

func TestDetailPaneBelowListOnNarrowTerminals(t *testing.T) {
	m := update(t, loaded(t, testConfig()), tea.WindowSizeMsg{Width: 70, Height: 30})
	v := view(m)
	if !strings.Contains(v, strings.Repeat("─", 70)) {
		t.Errorf("expected a rule between list and detail, got:\n%s", v)
	}
	if !strings.Contains(v, "Commands\n  go build ./...") {
		t.Errorf("expected build's detail below the list, got:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n != 30 {
		t.Errorf("expected 30 lines, got %d", n)
	}

	m = update(t, m, tea.WindowSizeMsg{Width: 70, Height: 10})
	if strings.Contains(view(m), "─") {
		t.Errorf("expected no detail pane on a short terminal, got:\n%s", view(m))
	}
}

func TestDetailPaneFollowsShells(t *testing.T) {
	m := update(t, loaded(t, testConfig()), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = press(t, m, "tab", "down")
	if v := view(m); !strings.Contains(v, "Extends      ci → minimal") {
		t.Errorf("expected default's detail, got:\n%s", v)
	}
}

func TestDetailPaneIsClipped(t *testing.T) {
	cfg := testConfig()
	var cmds []string
	for i := range 100 {
		cmds = append(cmds, fmt.Sprintf("echo %d", i))
	}
	cfg.Tasks["build"] = config.Task{Commands: cmds}
	m := update(t, loaded(t, cfg), tea.WindowSizeMsg{Width: 120, Height: 20})
	v := view(m)
	if n := strings.Count(v, "\n") + 1; n != 20 {
		t.Errorf("expected 20 lines, got %d", n)
	}
	if !strings.Contains(v, "│ …") {
		t.Errorf("expected the clipped detail to end with …, got:\n%s", v)
	}
}
