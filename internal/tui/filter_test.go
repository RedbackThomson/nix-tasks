package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		typed string
		want  []string
	}{
		{"", []string{"build", "lint", "test"}},
		{"lin", []string{"lint"}},
		{"LINT", []string{"lint"}},
		{"tests", []string{"test"}},
		{"the", []string{"build", "lint"}},
		{"nothing", nil},
	}
	for _, tt := range tests {
		t.Run(tt.typed, func(t *testing.T) {
			m := press(t, loaded(t, testConfig()), "/")
			for _, r := range tt.typed {
				m = press(t, m, string(r))
			}
			if got := m.visible(); !slices.Equal(got, tt.want) {
				t.Errorf("visible = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterKeys(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "/", "l", "i", "n", "j")
	if m.filter != "linj" {
		t.Fatalf("expected j to be typed into the filter, got %q", m.filter)
	}
	if v := view(m); !strings.Contains(v, "/linj█") || !strings.Contains(v, `No tasks match "linj"`) {
		t.Errorf("expected the filter and no matches, got:\n%s", v)
	}

	m = press(t, m, "backspace")
	if v := view(m); !strings.Contains(v, "/lin█  1 match") || !strings.Contains(cursorLine(t, m), "lint") {
		t.Errorf("expected lint to match, got:\n%s", v)
	}

	m = press(t, m, "enter")
	if m.filtering || m.filter != "lin" {
		t.Fatalf("expected enter to keep the filter, got filtering=%v filter=%q", m.filtering, m.filter)
	}
	if v := view(m); strings.Contains(v, "█") || !strings.Contains(v, "/lin") {
		t.Errorf("expected the kept filter without an input cursor, got:\n%s", v)
	}

	m = press(t, m, "esc")
	if m.filter != "" || len(m.visible()) != 3 {
		t.Errorf("expected esc to clear the filter, got %q", m.filter)
	}

	m = press(t, m, "/", "x", "esc")
	if m.filtering || m.filter != "" {
		t.Errorf("expected esc while typing to clear and stop, got filtering=%v filter=%q", m.filtering, m.filter)
	}
}

func TestFilterKeepsSelection(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "end", "/", "t")
	if line := cursorLine(t, m); !strings.Contains(line, "test") {
		t.Errorf("expected test to stay selected, got %q", line)
	}
	m = press(t, m, "e", "s", "t", "s", "esc")
	if line := cursorLine(t, m); !strings.Contains(line, "test") {
		t.Errorf("expected test to stay selected after clearing, got %q", line)
	}
}

func TestFilterLongNames(t *testing.T) {
	cfg := &config.Config{Tasks: map[string]config.Task{
		"integration-tests-for-the-payments-service-against-staging-db": {},
		"integration-tests-for-the-ledger-service-against-staging-db":   {},
		"unit": {},
	}}
	m := update(t, loaded(t, cfg), tea.WindowSizeMsg{Width: 50, Height: 20})
	m = press(t, m, "/")
	for _, r := range "ledger" {
		m = press(t, m, string(r))
	}
	if got := m.visible(); len(got) != 1 || !strings.Contains(got[0], "ledger") {
		t.Errorf("visible = %v", got)
	}
}

func TestTabClearsFilter(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "/", "b", "enter", "tab")
	if m.filter != "" || len(m.visible()) != 3 {
		t.Errorf("expected the shell list unfiltered, got filter %q", m.filter)
	}
}

func TestEnterAfterFilterRunsMatch(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "/", "l", "i", "enter")
	if name, _ := m.selected(); name != "lint" {
		t.Errorf("expected lint selected, got %q", name)
	}
	if _, cmd := pressCmd(t, m, "enter"); cmd == nil {
		t.Error("expected a second enter to run the match")
	}
}
