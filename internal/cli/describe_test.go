package cli

import (
	"bytes"
	"strings"
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

func TestDescribeTask_Label(t *testing.T) {
	ui.SetColor(false)
	const long = "test-unit-crossplane-provider-aws-ec2-managed-resources"
	graph, err := runner.NewTaskGraph(map[string]config.Task{
		"lint":  {},
		"build": {Label: "b"},
		long:    {},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"lint":  "",
		"build": "Label:\n  b\n\n",
		long:    "Label:\n  t-u-c-p-a-e-managed-resources (abbreviated; set `label` to choose one)\n\n",
	} {
		var buf bytes.Buffer
		describeTask(&buf, graph, name)
		got := buf.String()
		if want == "" {
			if strings.Contains(got, "Label:") {
				t.Errorf("%s: expected no label when it is the name:\n%s", name, got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s: expected %q in:\n%s", name, want, got)
		}
	}
}
