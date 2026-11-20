package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

func TestGraphView(t *testing.T) {
	defer ui.SetColor(ui.ColorEnabled())
	ui.SetColor(false)

	m := press(t, loaded(t, testConfig()), "end", "g")
	if !m.graphView {
		t.Fatal("expected g to open the graph")
	}
	v := view(m)
	if !strings.Contains(v, "Graph of test\ntest Run tests\n  build Build the app") {
		t.Errorf("expected test's graph, got:\n%s", v)
	}

	if m = press(t, m, "esc"); m.graphView {
		t.Error("expected esc to return to the list")
	}
	if line := cursorLine(t, m); !strings.Contains(line, "test") {
		t.Errorf("expected the selection kept, got %q", line)
	}
}

func TestGraphViewScrolls(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{}}
	var deps []string
	for i := range 40 {
		name := fmt.Sprintf("dep-%02d", i)
		cfg.Tasks[name] = config.Task{}
		deps = append(deps, name)
	}
	cfg.Tasks["all"] = config.Task{Depends: deps}
	m := update(t, loaded(t, cfg), tea.WindowSizeMsg{Width: 80, Height: 12})
	m = press(t, m, "g")

	if v := view(m); !strings.Contains(v, "dep-00") || strings.Contains(v, "dep-39") {
		t.Errorf("expected the top of the graph, got:\n%s", v)
	}
	m = press(t, m, "end")
	if v := view(m); !strings.Contains(v, "dep-39") || strings.Contains(v, "dep-00") {
		t.Errorf("expected the bottom of the graph, got:\n%s", v)
	}
	m = press(t, m, "pgdown", "down")
	if v := view(m); !strings.Contains(v, "dep-39") {
		t.Errorf("expected scrolling to stop at the end, got:\n%s", v)
	}
	m = press(t, m, "home", "up")
	if v := view(m); !strings.Contains(v, "Graph of all\nall\n  dep-00") {
		t.Errorf("expected scrolling to stop at the top, got:\n%s", v)
	}
	if n := strings.Count(view(m), "\n") + 1; n != 12 {
		t.Errorf("expected 12 lines, got %d", n)
	}
}

func TestGraphOnlyForTasks(t *testing.T) {
	if m := press(t, loaded(t, testConfig()), "tab", "g"); m.graphView {
		t.Error("expected g to do nothing on dev shells")
	}
}

func TestGraphShowsGraphErrors(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{"a": {Depends: []string{"missing"}}}}
	m := press(t, loaded(t, cfg), "g")
	if v := view(m); !strings.Contains(v, "missing") {
		t.Errorf("expected the graph error, got:\n%s", v)
	}
}
