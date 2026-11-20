package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/runner"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// Mode is the list being shown.
type Mode int

const (
	TaskMode Mode = iota
	ShellMode
)

// Options configures the TUI.
type Options struct {
	// Load evaluates the flake's configuration. It runs while the UI shows a
	// spinner, so it may be slow.
	Load func() (*config.Config, error)
	// FlakePath is shown while the configuration loads.
	FlakePath string
	// Executable and Args are the nix-tasks binary and global flags used to
	// run tasks and enter shells, so they behave as they would from the
	// command line.
	Executable string
	Args       []string
}

// Model is the Bubble Tea model for the TUI.
type Model struct {
	opts Options

	loading bool
	loadErr error
	spinner int

	config   *config.Config
	tasks    []string
	shells   []string
	graph    *runner.TaskGraph
	graphErr error

	mode   Mode
	cursor int
	offset int
	width  int
	height int

	// filter narrows the active list; filtering is set while it is typed.
	filter    string
	filtering bool

	// marked holds the tasks enter runs together.
	marked map[string]bool

	showHelp bool
	status   string

	// graphView shows graphLines, the dependency tree of graphTask.
	graphView   bool
	graphTask   string
	graphLines  []string
	graphOffset int
}

type configLoadedMsg struct {
	config *config.Config
	err    error
}

type spinnerTickMsg struct{}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 100 * time.Millisecond

// New returns a model that starts loading the configuration when it runs.
func New(opts Options) Model {
	return Model{opts: opts, loading: true, marked: map[string]bool{}}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadConfig, spinnerTick())
}

func (m Model) loadConfig() tea.Msg {
	cfg, err := m.opts.Load()
	return configLoadedMsg{config: cfg, err: err}
}

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case configLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		m.setConfig(msg.config)
		return m, nil

	case spinnerTickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinner = (m.spinner + 1) % len(spinnerFrames)
		return m, spinnerTick()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollToCursor()
		return m, nil

	case runFinishedMsg:
		m.status = runStatus(msg)
		return m, nil

	case shellFinishedMsg:
		m.status = shellStatus(msg)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) setConfig(cfg *config.Config) {
	m.config = cfg
	m.tasks = sortedKeys(cfg.Tasks)
	m.shells = sortedKeys(cfg.DevShells)
	m.graph, m.graphErr = runner.NewTaskGraph(cfg.Tasks)
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.loading || m.loadErr != nil {
		if key == "q" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.showHelp {
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}
	if m.graphView {
		return m.handleGraphKey(key), nil
	}
	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "/":
		m.filtering = true
	case "esc":
		if m.filter != "" {
			m.setFilter("")
		} else {
			clear(m.marked)
		}
	case "space":
		m.toggleMark()
	case "g":
		m.openGraph()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+b":
		m.moveCursor(-m.listHeight())
	case "pgdown", "ctrl+f":
		m.moveCursor(m.listHeight())
	case "home":
		m.moveCursor(-len(m.visible()))
	case "end":
		m.moveCursor(len(m.visible()))
	case "tab", "shift+tab":
		m.switchMode()
	case "enter", "r":
		return m, m.runSelected()
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.setFilter("")
	case "enter":
		m.filtering = false
	case "backspace":
		runes := []rune(m.filter)
		if len(runes) > 0 {
			m.setFilter(string(runes[:len(runes)-1]))
		}
	case "up":
		m.moveCursor(-1)
	case "down":
		m.moveCursor(1)
	default:
		if msg.Text != "" && msg.Mod&^tea.ModShift == 0 {
			m.setFilter(m.filter + msg.Text)
		}
	}
	return m, nil
}

// setFilter changes the filter, keeping the selected item selected when it
// still matches.
func (m *Model) setFilter(filter string) {
	selected, _ := m.selected()
	m.filter = filter
	m.cursor = max(0, slices.Index(m.visible(), selected))
	m.offset = 0
	m.scrollToCursor()
}

// visible returns the names in the active list that match the filter.
func (m Model) visible() []string {
	items := m.tasks
	if m.mode == ShellMode {
		items = m.shells
	}
	if m.filter == "" {
		return items
	}
	filter := strings.ToLower(m.filter)
	var matched []string
	for _, name := range items {
		if strings.Contains(strings.ToLower(name), filter) ||
			strings.Contains(strings.ToLower(m.summary(name)), filter) {
			matched = append(matched, name)
		}
	}
	return matched
}

func (m *Model) openGraph() {
	name, ok := m.selected()
	if m.mode != TaskMode || !ok {
		return
	}
	m.graphView, m.graphTask, m.graphOffset = true, name, 0
	if m.graphErr != nil {
		m.graphLines = strings.Split(m.graphErr.Error(), "\n")
		return
	}
	lines, err := ui.GraphTree(m.graph, name)
	if err != nil {
		lines = strings.Split(err.Error(), "\n")
	}
	m.graphLines = lines
}

func (m Model) handleGraphKey(key string) Model {
	page := m.graphRows()
	switch key {
	case "esc", "q", "g":
		m.graphView = false
		return m
	case "up", "k":
		m.graphOffset--
	case "down", "j":
		m.graphOffset++
	case "pgup", "ctrl+b":
		m.graphOffset -= page
	case "pgdown", "ctrl+f":
		m.graphOffset += page
	case "home":
		m.graphOffset = 0
	case "end":
		m.graphOffset = len(m.graphLines)
	}
	m.graphOffset = max(0, min(m.graphOffset, len(m.graphLines)-page))
	return m
}

func (m Model) selected() (string, bool) {
	items := m.visible()
	if m.cursor < 0 || m.cursor >= len(items) {
		return "", false
	}
	return items[m.cursor], true
}

func (m *Model) moveCursor(delta int) {
	m.cursor = max(0, min(m.cursor+delta, len(m.visible())-1))
	m.scrollToCursor()
}

// scrollToCursor moves the list's window just enough to keep the cursor on
// screen.
func (m *Model) scrollToCursor() {
	rows := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, len(m.visible())-rows))
}

func (m *Model) switchMode() {
	if m.mode == TaskMode {
		m.mode = ShellMode
	} else {
		m.mode = TaskMode
	}
	m.cursor = 0
	m.offset = 0
	m.filter = ""
	m.filtering = false
}

func (m *Model) toggleMark() {
	name, ok := m.selected()
	if m.mode != TaskMode || !ok {
		return
	}
	if m.marked[name] {
		delete(m.marked, name)
	} else {
		m.marked[name] = true
	}
	m.moveCursor(1)
}

// markedTasks returns the marked tasks in list order.
func (m Model) markedTasks() []string {
	var names []string
	for _, name := range m.tasks {
		if m.marked[name] {
			names = append(names, name)
		}
	}
	return names
}

// runSelected runs the marked tasks, or else the selected one, or enters the
// selected dev shell.
func (m Model) runSelected() tea.Cmd {
	if m.mode == ShellMode {
		if name, ok := m.selected(); ok {
			return m.enterShell(name)
		}
		return nil
	}
	if names := m.tasksToRun(); len(names) > 0 {
		return m.runTasks(names)
	}
	return nil
}

func (m Model) tasksToRun() []string {
	if marked := m.markedTasks(); len(marked) > 0 {
		return marked
	}
	if name, ok := m.selected(); ok {
		return []string{name}
	}
	return nil
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// Run shows the TUI until the user quits.
func Run(opts Options) error {
	var progOpts []tea.ProgramOption
	if !ui.ColorEnabled() {
		progOpts = append(progOpts, tea.WithColorProfile(colorprofile.Ascii))
	}
	if _, err := tea.NewProgram(New(opts), progOpts...).Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}
	return nil
}
