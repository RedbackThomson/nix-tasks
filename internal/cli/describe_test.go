package cli

import (
	"bytes"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

func TestDescribeTask(t *testing.T) {
	ui.SetColor(false)
	graph, err := runner.NewTaskGraph(map[string]config.Task{
		"gen":    {},
		"build":  {Description: "Build it", Depends: []string{"gen"}, Commands: []string{"go build ./..."}},
		"test":   {Depends: []string{"build"}},
		"lint":   {Depends: []string{"build"}},
		"notify": {After: []string{"build"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	describeTask(&buf, graph, "build")

	want := `Task: build

Description:
  Build it

Depends on:
  - gen

Depended on by:
  - lint
  - test

After-hooked by:
  - notify

Commands:
  go build ./...
`
	if got := buf.String(); got != want {
		t.Errorf("describeTask() =\n%s\nwant\n%s", got, want)
	}
}
