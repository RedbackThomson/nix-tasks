package tui

import (
	"fmt"
	"slices"
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
	markStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(ui.AccentColor))
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

// Terminals at least this wide show the detail pane beside the list rather
// than below it.
const sideBySideWidth = 100

// paneSeparator goes between the list and the detail pane beside it.
const paneSeparator = " │ "

// layout is where the parts of the main screen go.
type layout struct {
	listWidth, listRows     int
	detailWidth, detailRows int
	side                    bool
}

// The main screen has a header line, an optional filter line, the body, a
// status line and a key hint line.
func (m Model) layout() layout {
	width, height := m.size()
	body := height - 3
	if m.filtering || m.filter != "" {
		body--
	}
	body = max(1, body)

	if width >= sideBySideWidth {
		listWidth := width * 2 / 5
		return layout{
			listWidth: listWidth, listRows: body,
			detailWidth: width - listWidth - len([]rune(paneSeparator)), detailRows: body,
			side: true,
		}
	}
	// Below the list, with a rule between them, when there is room for both.
	if body < 10 {
		return layout{listWidth: width, listRows: body}
	}
	listRows := max(3, min(body/2, len(m.visible())))
	return layout{listWidth: width, listRows: listRows, detailWidth: width, detailRows: body - listRows - 1}
}

// listHeight is how many list rows fit on screen.
func (m Model) listHeight() int {
	return m.layout().listRows
}

func (m Model) graphRows() int {
	_, height := m.size()
	return max(1, height-2)
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
	case m.graphView:
		lines = m.graphViewLines()
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
	l := m.layout()
	lines := []string{m.header()}
	if m.filtering || m.filter != "" {
		lines = append(lines, m.filterLine())
	}

	list := m.listLines(l.listWidth, l.listRows)
	detail := clip(m.detailLines(l.detailWidth), l.detailRows)
	switch {
	case l.side:
		for i, row := range list {
			pad := strings.Repeat(" ", max(0, l.listWidth-ansi.StringWidth(row)))
			right := ""
			if i < len(detail) {
				right = detail[i]
			}
			lines = append(lines, row+pad+dimStyle.Render(paneSeparator)+right)
		}
	case l.detailRows > 0:
		lines = append(lines, list...)
		lines = append(lines, dimStyle.Render(strings.Repeat("─", l.detailWidth)))
		lines = append(lines, padLines(detail, l.detailRows)...)
	default:
		lines = append(lines, list...)
	}

	return append(lines, m.statusLine(), dimStyle.Render(m.keyHints()))
}

func (m Model) statusLine() string {
	n := len(m.marked)
	if n == 0 {
		return m.status
	}
	marks := markStyle.Render(fmt.Sprintf("● %d marked", n))
	if m.status == "" {
		return marks
	}
	return marks + dimStyle.Render(" · ") + m.status
}

func (m Model) filterLine() string {
	line := "/" + m.filter
	if m.filtering {
		line += "█"
	}
	n := len(m.visible())
	return line + dimStyle.Render(fmt.Sprintf("  %d %s", n, plural(n, "match", "matches")))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (m Model) detailLines(width int) []string {
	name, ok := m.selected()
	if !ok || width <= 0 {
		return nil
	}
	if m.mode == ShellMode {
		return shellDetail(name, m.config.DevShells, width)
	}
	return taskDetail(name, m.config.Tasks[name], width)
}

// clip cuts lines to rows, marking that some were left out.
func clip(lines []string, rows int) []string {
	if len(lines) <= rows {
		return lines
	}
	if rows <= 0 {
		return nil
	}
	return append(lines[:rows-1:rows-1], dimStyle.Render("…"))
}

func padLines(lines []string, rows int) []string {
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) graphViewLines() []string {
	rows := m.graphRows()
	lines := []string{titleStyle.Render("Graph of " + m.graphTask)}
	end := min(len(m.graphLines), m.graphOffset+rows)
	lines = append(lines, padLines(slices.Clone(m.graphLines[m.graphOffset:end]), rows)...)
	return append(lines, dimStyle.Render("↑/↓ scroll · esc back · * already shown above"))
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

func (m Model) listLines(width, rows int) []string {
	items := m.visible()
	lines := make([]string, 0, rows)
	if len(items) == 0 {
		what := "tasks"
		if m.mode == ShellMode {
			what = "dev shells"
		}
		if m.filter != "" {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  No %s match %q", what, m.filter)))
		} else {
			lines = append(lines, dimStyle.Render("  No "+what+" defined"))
		}
	}
	for i := m.offset; i < len(items) && i < m.offset+rows; i++ {
		lines = append(lines, ansi.Truncate(m.itemLine(items[i], i == m.cursor), width, "…"))
	}
	return padLines(lines, rows)
}

func (m Model) itemLine(name string, selected bool) string {
	cursor, label := "  ", name
	if selected {
		cursor, label = selectedStyle.Render("> "), selectedStyle.Render(name)
	}
	line := cursor
	if m.mode == TaskMode {
		mark := "  "
		if m.marked[name] {
			mark = markStyle.Render("● ")
		}
		line += mark
	}
	line += label
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
	if m.filtering {
		return "type to filter · enter keep · esc clear"
	}
	if m.mode == ShellMode {
		return "enter enter shell · / filter · tab tasks · ? help · q quit"
	}
	if len(m.marked) > 0 {
		return "enter run marked · space mark · esc unmark all · ? help · q quit"
	}
	return "enter run · space mark · / filter · g graph · tab shells · ? help · q quit"
}

var keyHelp = [][2]string{
	{"↑/k ↓/j", "move"},
	{"pgup pgdown", "move a page (also ctrl+b, ctrl+f)"},
	{"home end", "first or last item"},
	{"tab", "switch between tasks and dev shells"},
	{"enter, r", "run the task, or every marked task in one run (or enter the dev shell)"},
	{"space", "mark or unmark the task"},
	{"/", "filter by name or description; enter keeps it, esc clears it"},
	{"esc", "clear the filter, or else unmark every task"},
	{"g", "show the task's dependency graph (esc to return)"},
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
