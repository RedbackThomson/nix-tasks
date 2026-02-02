package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// Styles for the TUI
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	sectionInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)
)

// Mode represents the current section being viewed
type Mode int

const (
	TaskMode Mode = iota
	ShellMode
)

// Model is the Bubbletea model for the TUI
type Model struct {
	config    *config.Config
	tasks     []string
	shells    []string
	cursor    int
	mode      Mode
	width     int
	height    int
	flakePath string
	quitting  bool
	runTask   string
	runShell  string
}

// Tasks returns the list of task names
func (m Model) Tasks() []string {
	return m.tasks
}

// Shells returns the list of shell names
func (m Model) Shells() []string {
	return m.shells
}

// Cursor returns the current cursor position
func (m Model) Cursor() int {
	return m.cursor
}

// CurrentMode returns the current mode (TaskMode or ShellMode)
func (m Model) CurrentMode() Mode {
	return m.mode
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config, flakePath string) Model {
	// Sort task names for consistent display
	tasks := make([]string, 0, len(cfg.Tasks))
	for name := range cfg.Tasks {
		tasks = append(tasks, name)
	}
	sort.Strings(tasks)

	// Sort shell names for consistent display
	shells := make([]string, 0, len(cfg.DevShells))
	for name := range cfg.DevShells {
		shells = append(shells, name)
	}
	sort.Strings(shells)

	return Model{
		config:    cfg,
		tasks:     tasks,
		shells:    shells,
		mode:      TaskMode,
		flakePath: flakePath,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			max := m.maxCursor()
			if m.cursor < max {
				m.cursor++
			}

		case "tab":
			m.switchMode()

		case "enter", "r":
			return m, m.runSelected()

		case "?":
			// Could show help overlay
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("nix-tasks"))
	b.WriteString("\n\n")

	// Tasks section
	m.renderTasksSection(&b)
	b.WriteString("\n")

	// Shells section
	m.renderShellsSection(&b)
	b.WriteString("\n")

	// Help
	b.WriteString(helpStyle.Render("[j/k] navigate  [tab] switch section  [enter/r] run  [q] quit"))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderTasksSection(b *strings.Builder) {
	// Section header
	header := "Tasks"
	if m.mode == TaskMode {
		b.WriteString(sectionStyle.Render(header))
	} else {
		b.WriteString(sectionInactiveStyle.Render(header))
	}
	b.WriteString("\n")

	if len(m.tasks) == 0 {
		b.WriteString(descStyle.Render("  No tasks defined"))
		b.WriteString("\n")
		return
	}

	for i, name := range m.tasks {
		task := m.config.Tasks[name]

		// Cursor
		cursor := "  "
		style := normalStyle
		if m.mode == TaskMode && i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedStyle
		}

		// Task name
		line := cursor + style.Render(name)

		// Description
		if task.Description != "" {
			desc := descStyle.Render(" - " + task.Description)
			line += desc
		}

		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (m Model) renderShellsSection(b *strings.Builder) {
	// Section header
	header := "Dev Shells"
	if m.mode == ShellMode {
		b.WriteString(sectionStyle.Render(header))
	} else {
		b.WriteString(sectionInactiveStyle.Render(header))
	}
	b.WriteString("\n")

	if len(m.shells) == 0 {
		b.WriteString(descStyle.Render("  No shells defined"))
		b.WriteString("\n")
		return
	}

	for i, name := range m.shells {
		shell := m.config.DevShells[name]

		// Cursor
		cursor := "  "
		style := normalStyle
		if m.mode == ShellMode && i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedStyle
		}

		// Shell name
		line := cursor + style.Render(name)

		// Extends info
		if shell.Extends != "" {
			desc := descStyle.Render(" (extends " + shell.Extends + ")")
			line += desc
		}

		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (m Model) maxCursor() int {
	if m.mode == TaskMode {
		if len(m.tasks) == 0 {
			return 0
		}
		return len(m.tasks) - 1
	}
	if len(m.shells) == 0 {
		return 0
	}
	return len(m.shells) - 1
}

func (m *Model) switchMode() {
	if m.mode == TaskMode {
		m.mode = ShellMode
	} else {
		m.mode = TaskMode
	}
	m.cursor = 0
}

func (m Model) runSelected() tea.Cmd {
	if m.mode == TaskMode && len(m.tasks) > 0 {
		taskName := m.tasks[m.cursor]
		return tea.ExecProcess(
			exec.Command("nix-tasks", "-f", m.flakePath, "run", taskName),
			func(err error) tea.Msg {
				return taskFinishedMsg{err: err}
			},
		)
	} else if m.mode == ShellMode && len(m.shells) > 0 {
		shellName := m.shells[m.cursor]
		return tea.ExecProcess(
			exec.Command("nix-tasks", "-f", m.flakePath, "shell", shellName),
			func(err error) tea.Msg {
				return shellFinishedMsg{err: err}
			},
		)
	}
	return nil
}

type taskFinishedMsg struct {
	err error
}

type shellFinishedMsg struct {
	err error
}

// Run starts the TUI
func Run(cfg *config.Config, flakePath string) error {
	model := NewModel(cfg, flakePath)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	// Check if we need to do anything after TUI exits
	if m, ok := finalModel.(Model); ok {
		if m.runTask != "" {
			// Run the task
			cmd := exec.Command("nix-tasks", "-f", flakePath, "run", m.runTask)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		if m.runShell != "" {
			// Enter the shell
			cmd := exec.Command("nix-tasks", "-f", flakePath, "shell", m.runShell)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}

	return nil
}
