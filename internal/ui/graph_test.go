package ui

import (
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestGraphTree(t *testing.T) {
	defer SetColor(ColorEnabled())
	SetColor(false)

	graph, err := runner.NewTaskGraph(map[string]config.Task{
		"compile": {},
		"lint":    {Description: "Lint"},
		"build":   {Description: "Build it", Depends: []string{"compile", "lint"}},
		"test":    {Depends: []string{"build"}},
		"deploy":  {Depends: []string{"build", "test"}},
		"notify":  {After: []string{"deploy"}},
		"unused":  {After: []string{"other"}},
		"other":   {},
	})
	if err != nil {
		t.Fatal(err)
	}

	lines, err := GraphTree(graph, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"deploy",
		"  build Build it",
		"    compile",
		"    lint Lint",
		"  test",
		"    build Build it *",
		"  notify (after)",
	}
	if got := strings.Join(lines, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("GraphTree() =\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}

	if _, err := GraphTree(graph, "missing"); err == nil {
		t.Error("expected an error for an unknown task")
	}
}
