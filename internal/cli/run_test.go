package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestRunAcceptsSeveralTasks(t *testing.T) {
	var c CLI
	parser, err := kong.New(&c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"run", "build", "test", "lint", "-j", "2"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"build", "test", "lint"}; !slices.Equal(c.Run.Tasks, want) {
		t.Errorf("Tasks = %v, want %v", c.Run.Tasks, want)
	}
	if _, err := parser.Parse([]string{"run"}); err == nil {
		t.Error("expected run without a task to be rejected")
	}
}

func TestRunRawTakesOneTask(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"run", "build", "--raw"}, false},
		{[]string{"run", "build", "test", "--raw"}, true},
	} {
		var c CLI
		parser, err := kong.New(&c)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parser.Parse(tt.args)
		if (err != nil) != tt.wantErr || (err != nil && !strings.Contains(err.Error(), "--raw")) {
			t.Errorf("%v: error = %v, wantErr %v", tt.args, err, tt.wantErr)
		}
	}
}

func TestCheckTasksExist(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{"build": {}, "test": {}}}
	if err := checkTasksExist(cfg, []string{"build", "test"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	err := checkTasksExist(cfg, []string{"build", "lint", "deploy"})
	if err == nil || err.Error() != "task not found: lint, deploy" {
		t.Errorf("got %v, want an error naming lint and deploy", err)
	}
}
