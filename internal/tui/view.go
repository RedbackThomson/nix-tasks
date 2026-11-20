package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/ui"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	activeTab     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(ui.AccentColor))
	inactiveTab   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.MutedColor))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(ui.AccentColor))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.MutedColor))
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.LabelColor))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.SuccessColor))
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.FailureColor))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.WarningColor))
)

// Sizes used before the terminal reports its own.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

func (m Model) size() (width, height int) {
	width, height = m.width, m.height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}
	return width, height
}

// The main screen is a header line, the list, a status line and a key hint
// line.
const chromeLines = 3

// listHeight is how many list rows fit on screen.
func (m Model) listHeight() int {
	_, height := m.size()
	return max(1, height-chromeLines)
}

func (m Model) render() string {
	width, height := m.size()
	var lines []string
	switch {
	case m.loading:
		lines = []string{
			titleStyle.Render("nix-tasks"),
			"",
			fmt.Sprintf("%s Evaluating %s…", spinnerFrames[m.spinner], m.opts.FlakePath),
		}
	case m.loadErr != nil:
		lines = m.errorLines(width, height)
	case m.showHelp:
		lines = helpLines()
	default:
		lines = m.mainLines()
	}
	return fit(lines, width, height)
}

// fit truncates lines to the screen and pads them to its height, so nothing
// wraps and the layout never shifts.
func fit(lines []string, width, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = ansi.Truncate(lines[i], width, "…")
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) errorLines(width, height int) []string {
	head := []string{
		titleStyle.Render("nix-tasks"),
		"",
		failStyle.Render("✗ ") + "Could not load the configuration",
		"",
	}
	foot := []string{"", dimStyle.Render("q quit")}

	body := strings.Split(ansi.Wrap(m.loadErr.Error(), width, ""), "\n")
	// Nix puts the actual error last, after the evaluation trace.
	if room := height - len(head) - len(foot); len(body) > room {
		body = append([]string{dimStyle.Render("…")}, body[len(body)-room+1:]...)
	}
	return append(append(head, body...), foot...)
}

func (m Model) mainLines() []string {
	lines := []string{m.header()}
	lines = append(lines, m.listLines()...)
	lines = append(lines, m.status, dimStyle.Render(m.keyHints()))
	return lines
}

func (m Model) header() string {
	tab := func(mode Mode, label string, n int) string {
		text := fmt.Sprintf("%s (%d)", label, n)
		if m.mode == mode {
			return activeTab.Render(text)
		}
		return inactiveTab.Render(text)
	}
	return titleStyle.Render("nix-tasks") + "  " +
		tab(TaskMode, "Tasks", len(m.tasks)) + "  " +
		tab(ShellMode, "Dev Shells", len(m.shells))
}

func (m Model) listLines() []string {
	rows := m.listHeight()
	items := m.visible()
	lines := make([]string, 0, rows)
	if len(items) == 0 {
		what := "tasks"
		if m.mode == ShellMode {
			what = "dev shells"
		}
		lines = append(lines, dimStyle.Render("  No "+what+" defined"))
	}
	for i := m.offset; i < len(items) && i < m.offset+rows; i++ {
		lines = append(lines, m.itemLine(items[i], i == m.cursor))
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) itemLine(name string, selected bool) string {
	cursor, label := "  ", name
	if selected {
		cursor, label = selectedStyle.Render("> "), selectedStyle.Render(name)
	}
	line := cursor + label
	if desc := m.summary(name); desc != "" {
		line += "  " + dimStyle.Render(desc)
	}
	return line
}

// summary is the short text shown after a name in the list.
func (m Model) summary(name string) string {
	if m.mode == ShellMode {
		if parent := m.config.DevShells[name].Extends; parent != "" {
			return "extends " + parent
		}
		return ""
	}
	return firstLine(m.config.Tasks[name].Description)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func (m Model) keyHints() string {
	action := "run"
	if m.mode == ShellMode {
		action = "enter shell"
	}
	return "enter " + action + " · tab switch list · ? help · q quit"
}

var keyHelp = [][2]string{
	{"↑/k ↓/j", "move"},
	{"pgup pgdown", "move a page (also ctrl+b, ctrl+f)"},
	{"home end", "first or last item"},
	{"tab", "switch between tasks and dev shells"},
	{"enter, r", "run the task (or enter the dev shell)"},
	{"?", "show or hide this help"},
	{"q, ctrl+c", "quit"},
}

func helpLines() []string {
	lines := []string{titleStyle.Render("nix-tasks keys"), ""}
	for _, k := range keyHelp {
		lines = append(lines, fmt.Sprintf("  %-14s %s", k[0], k[1]))
	}
	return append(lines, "", dimStyle.Render("press ? or esc to close"))
}
