package tui

import (
	"slices"
	"strings"
	"testing"
)

func TestMarkingTasks(t *testing.T) {
	m := loaded(t, testConfig())
	if got := m.tasksToRun(); !slices.Equal(got, []string{"build"}) {
		t.Errorf("without marks expected the selected task, got %v", got)
	}

	m = press(t, m, "space", "down", "space")
	if line := cursorLine(t, m); !strings.Contains(line, "test") {
		t.Errorf("expected space to move down, got %q", line)
	}
	if got := m.tasksToRun(); !slices.Equal(got, []string{"build", "test"}) {
		t.Errorf("expected the marked tasks in list order, got %v", got)
	}
	v := view(m)
	if !strings.Contains(v, "● build") || !strings.Contains(v, "● test") || strings.Contains(v, "● lint") {
		t.Errorf("expected markers on build and test, got:\n%s", v)
	}
	if !strings.Contains(v, "● 2 marked") || !strings.Contains(v, "enter run marked") {
		t.Errorf("expected the mark count and hint, got:\n%s", v)
	}
	if _, cmd := pressCmd(t, m, "enter"); cmd == nil {
		t.Error("expected enter to run the marked tasks")
	}

	m = press(t, m, "space")
	if got := m.tasksToRun(); !slices.Equal(got, []string{"build"}) {
		t.Errorf("expected space to unmark test, got %v", got)
	}
}

func TestMarksSurviveFilteringAndEscClearsThem(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "space", "/", "l", "i", "n", "t", "enter", "space")
	if got := m.tasksToRun(); !slices.Equal(got, []string{"build", "lint"}) {
		t.Errorf("expected marks across filters, got %v", got)
	}

	m = press(t, m, "esc")
	if m.filter != "" || len(m.marked) != 2 {
		t.Errorf("expected the first esc to clear only the filter, got filter %q, %d marked", m.filter, len(m.marked))
	}
	m = press(t, m, "esc")
	if len(m.marked) != 0 {
		t.Errorf("expected the second esc to clear the marks, got %v", m.marked)
	}
}

func TestMarksStayAfterARun(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "space", "space")
	m = update(t, m, runFinishedMsg{names: m.tasksToRun()})
	if v := view(m); !strings.Contains(v, "● 2 marked · ✓ build, lint passed") {
		t.Errorf("expected the marks and the result, got:\n%s", v)
	}
}

func TestShellsCannotBeMarked(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "space", "tab", "space")
	if len(m.marked) != 1 || !m.marked["build"] {
		t.Errorf("expected only the task mark, got %v", m.marked)
	}
	if strings.Contains(cursorLine(t, m), "●") {
		t.Error("expected no mark column for shells")
	}
}
