package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestWarnings(t *testing.T) {
	var out bytes.Buffer
	w := &warnings{out: &out}
	w.add("first")
	w.add("second")
	if got := out.String(); got != "nix-tasks: warning: first\nnix-tasks: warning: second\n" {
		t.Errorf("printed %q", got)
	}
	if !slices.Equal(w.msgs, []string{"first", "second"}) {
		t.Errorf("collected %v", w.msgs)
	}
}

func TestStartRunLogs(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	w := &warnings{out: io.Discard}
	dir := startRunLogs(time.Now(), w)
	if filepath.Dir(dir) != filepath.Join(state, "nix-tasks", "runs") {
		t.Errorf("run logs in %q", dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("run log directory not created: %v", err)
	}
	if len(w.msgs) != 0 {
		t.Errorf("unexpected warnings %v", w.msgs)
	}

	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocked)
	if dir := startRunLogs(time.Now(), w); dir != "" || len(w.msgs) != 1 {
		t.Errorf("an unusable state dir should warn and keep no logs, got %q, %v", dir, w.msgs)
	}
}

func TestExitStatus(t *testing.T) {
	loadErr := errors.New("failed to load config")
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantErr  error
	}{
		{"success", nil, 0, nil},
		{"task exit code", &ExitCodeError{Code: 3, Err: errors.New("one or more tasks failed")}, 3, nil},
		{"interrupted", errInterrupted, exitInterrupted, nil},
		{"error outside the tasks", loadErr, 1, loadErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := exitStatus(tt.err)
			if code != tt.wantCode || err != tt.wantErr {
				t.Errorf("exitStatus() = %d, %v; want %d, %v", code, err, tt.wantCode, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsRawWithJSON(t *testing.T) {
	c := &RunCmd{Tasks: []string{"build"}, Raw: true}
	err := c.Run(&Globals{Output: OutputJSON})
	if err == nil || !strings.Contains(err.Error(), "--output=json cannot be combined with --raw") {
		t.Errorf("expected --raw to be rejected, got %v", err)
	}
}
