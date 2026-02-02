package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/tui"
)

func TestNewModel(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"build": {Description: "Build the app"},
			"test":  {Description: "Run tests"},
		},
		DevShells: map[string]config.Shell{
			"default": {Packages: []string{"go"}},
			"ci":      {Extends: "default"},
		},
	}

	model := tui.NewModel(cfg, ".")

	// Verify tasks are sorted
	if len(model.Tasks()) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(model.Tasks()))
	}
	if model.Tasks()[0] != "build" {
		t.Errorf("expected first task to be 'build', got '%s'", model.Tasks()[0])
	}

	// Verify shells are sorted
	if len(model.Shells()) != 2 {
		t.Errorf("expected 2 shells, got %d", len(model.Shells()))
	}
}

func TestModelNavigation(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"a": {},
			"b": {},
			"c": {},
		},
		DevShells: map[string]config.Shell{
			"default": {},
		},
	}

	model := tui.NewModel(cfg, ".")

	// Start at position 0
	if model.Cursor() != 0 {
		t.Errorf("expected cursor at 0, got %d", model.Cursor())
	}

	// Move down
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	m := newModel.(tui.Model)
	if m.Cursor() != 1 {
		t.Errorf("expected cursor at 1 after down, got %d", m.Cursor())
	}

	// Move down again
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newModel.(tui.Model)
	if m.Cursor() != 2 {
		t.Errorf("expected cursor at 2 after second down, got %d", m.Cursor())
	}

	// Can't move past the end
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newModel.(tui.Model)
	if m.Cursor() != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.Cursor())
	}

	// Move up
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = newModel.(tui.Model)
	if m.Cursor() != 1 {
		t.Errorf("expected cursor at 1 after up, got %d", m.Cursor())
	}
}

func TestModelModeSwitch(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"build": {},
		},
		DevShells: map[string]config.Shell{
			"default": {},
		},
	}

	model := tui.NewModel(cfg, ".")

	// Start in task mode
	if model.CurrentMode() != tui.TaskMode {
		t.Errorf("expected TaskMode, got %v", model.CurrentMode())
	}

	// Switch to shell mode with tab
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	m := newModel.(tui.Model)
	if m.CurrentMode() != tui.ShellMode {
		t.Errorf("expected ShellMode after tab, got %v", m.CurrentMode())
	}

	// Cursor should reset to 0
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after mode switch, got %d", m.Cursor())
	}

	// Switch back to task mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newModel.(tui.Model)
	if m.CurrentMode() != tui.TaskMode {
		t.Errorf("expected TaskMode after second tab, got %v", m.CurrentMode())
	}
}

func TestModelView(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"build": {Description: "Build the app"},
		},
		DevShells: map[string]config.Shell{
			"default": {},
		},
	}

	model := tui.NewModel(cfg, ".")
	view := model.View()

	// Check that key elements are present
	if view == "" {
		t.Error("expected non-empty view")
	}

	// Should contain the title
	if !contains(view, "nix-tasks") {
		t.Error("expected view to contain 'nix-tasks' title")
	}

	// Should contain the task name
	if !contains(view, "build") {
		t.Error("expected view to contain 'build' task")
	}

	// Should contain help text
	if !contains(view, "quit") {
		t.Error("expected view to contain help text")
	}
}

func TestModelVimKeys(t *testing.T) {
	cfg := &config.Config{
		Tasks: map[string]config.Task{
			"a": {},
			"b": {},
		},
		DevShells: map[string]config.Shell{},
	}

	model := tui.NewModel(cfg, ".")

	// Test 'j' for down
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m := newModel.(tui.Model)
	if m.Cursor() != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", m.Cursor())
	}

	// Test 'k' for up
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newModel.(tui.Model)
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after 'k', got %d", m.Cursor())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
