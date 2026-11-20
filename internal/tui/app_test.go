package tui

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Tasks: map[string]config.Task{
			"build": {Description: "Build the app", Deps: []string{"go"}, Commands: []string{"go build ./..."}},
			"test":  {Description: "Run tests", Depends: []string{"build"}},
			"lint":  {Description: "Lint the code"},
		},
		DevShells: map[string]config.Shell{
			"minimal": {Packages: []string{"bash"}},
			"ci":      {Extends: "minimal", Packages: []string{"go"}},
			"default": {Extends: "ci", Packages: []string{"kubectl"}, Env: map[string]string{"EDITOR": "vim"}},
		},
	}
}

var testOptions = Options{
	FlakePath:  "/src/app",
	Executable: "/nix/store/abc-nix-tasks/bin/nix-tasks",
	Args:       []string{"-f", "/src/app", "-v"},
}

func loaded(t *testing.T, cfg *config.Config) Model {
	t.Helper()
	return update(t, New(testOptions), configLoadedMsg{config: cfg})
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		m = update(t, m, key(k))
	}
	return m
}

func pressCmd(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key(k))
	return next.(Model), cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

// cursorLine returns the list line the cursor is on.
func cursorLine(t *testing.T, m Model) string {
	t.Helper()
	for _, line := range strings.Split(view(m), "\n") {
		if strings.HasPrefix(line, "> ") {
			return line
		}
	}
	t.Fatalf("no cursor line in view:\n%s", view(m))
	return ""
}

func TestLoadingShowsSpinnerUntilConfigArrives(t *testing.T) {
	m := New(testOptions)
	if !strings.Contains(view(m), "Evaluating /src/app") {
		t.Errorf("expected a loading message, got:\n%s", view(m))
	}
	if m.Init() == nil {
		t.Fatal("expected Init to start loading")
	}

	ticked := update(t, m, spinnerTickMsg{})
	if ticked.spinner == m.spinner {
		t.Error("expected the spinner to advance")
	}

	m = update(t, m, configLoadedMsg{config: testConfig()})
	if strings.Contains(view(m), "Evaluating") {
		t.Errorf("expected the loading message to go, got:\n%s", view(m))
	}
	for _, name := range []string{"build", "lint", "test"} {
		if !strings.Contains(view(m), name) {
			t.Errorf("expected %s in the list, got:\n%s", name, view(m))
		}
	}
	if _, cmd := m.Update(spinnerTickMsg{}); cmd != nil {
		t.Error("expected the spinner to stop once loaded")
	}
}

func TestLoadRunsTheLoader(t *testing.T) {
	cfg := testConfig()
	opts := testOptions
	opts.Load = func() (*config.Config, error) { return cfg, nil }
	msg := New(opts).loadConfig()
	if got, ok := msg.(configLoadedMsg); !ok || got.config != cfg {
		t.Errorf("expected the loaded config, got %#v", msg)
	}
}

func TestLoadErrorIsShownInTheUI(t *testing.T) {
	m := update(t, New(testOptions), configLoadedMsg{err: errors.New("failed to load config: error: undefined variable 'pkgs'")})

	v := view(m)
	if !strings.Contains(v, "Could not load the configuration") || !strings.Contains(v, "undefined variable 'pkgs'") {
		t.Errorf("expected the error in the view, got:\n%s", v)
	}
	if _, cmd := pressCmd(t, m, "q"); !isQuit(cmd) {
		t.Error("expected q to quit")
	}
}

func TestLongLoadErrorKeepsItsEnd(t *testing.T) {
	var trace []string
	for i := range 100 {
		trace = append(trace, fmt.Sprintf("… while evaluating frame %d", i))
	}
	trace = append(trace, "error: the real problem")
	m := update(t, New(testOptions), configLoadedMsg{err: errors.New(strings.Join(trace, "\n"))})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})

	v := view(m)
	if !strings.Contains(v, "the real problem") {
		t.Errorf("expected the last error line, got:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n != 20 {
		t.Errorf("expected 20 lines, got %d", n)
	}
}

func TestNavigation(t *testing.T) {
	m := loaded(t, testConfig())
	if line := cursorLine(t, m); !strings.Contains(line, "build") {
		t.Errorf("expected the cursor on build, got %q", line)
	}

	tests := []struct {
		keys []string
		want string
	}{
		{[]string{"down"}, "lint"},
		{[]string{"j", "j"}, "test"},
		{[]string{"down", "down", "down"}, "test"},
		{[]string{"down", "up"}, "build"},
		{[]string{"j", "k", "k"}, "build"},
		{[]string{"end"}, "test"},
		{[]string{"end", "home"}, "build"},
		{[]string{"pgdown"}, "test"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, " "), func(t *testing.T) {
			got := press(t, m, tt.keys...)
			if line := cursorLine(t, got); !strings.Contains(line, tt.want) {
				t.Errorf("expected the cursor on %s, got %q", tt.want, line)
			}
		})
	}
}

func TestTabSwitchesToShells(t *testing.T) {
	m := press(t, loaded(t, testConfig()), "down", "tab")
	if m.mode != ShellMode {
		t.Fatal("expected tab to show dev shells")
	}
	if line := cursorLine(t, m); !strings.Contains(line, "ci") || !strings.Contains(line, "extends minimal") {
		t.Errorf("expected the cursor on the first shell, got %q", line)
	}
	if m = press(t, m, "tab"); m.mode != TaskMode {
		t.Error("expected a second tab to show tasks")
	}
}

func manyTasks(n int) *config.Config {
	cfg := &config.Config{Tasks: map[string]config.Task{}}
	for i := range n {
		cfg.Tasks[fmt.Sprintf("task-%02d", i)] = config.Task{Description: fmt.Sprintf("Task number %d", i)}
	}
	return cfg
}

func TestScrollingKeepsCursorVisible(t *testing.T) {
	m := update(t, loaded(t, manyTasks(50)), tea.WindowSizeMsg{Width: 80, Height: 12})

	for i := range 49 {
		m = press(t, m, "down")
		want := fmt.Sprintf("task-%02d", i+1)
		if line := cursorLine(t, m); !strings.Contains(line, want) {
			t.Fatalf("after %d downs expected the cursor on %s, got %q", i+1, want, line)
		}
		if n := strings.Count(view(m), "\n") + 1; n != 12 {
			t.Fatalf("expected the view to fill 12 lines, got %d", n)
		}
	}
	if strings.Contains(view(m), "task-00") {
		t.Error("expected the top of the list to scroll away")
	}

	m = press(t, m, "home")
	if line := cursorLine(t, m); !strings.Contains(line, "task-00") {
		t.Errorf("expected home to scroll back up, got %q", line)
	}
}

func TestResizeKeepsCursorVisible(t *testing.T) {
	m := update(t, loaded(t, manyTasks(50)), tea.WindowSizeMsg{Width: 80, Height: 40})
	m = press(t, m, "end")
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	if line := cursorLine(t, m); !strings.Contains(line, "task-49") {
		t.Errorf("expected the cursor to stay visible after shrinking, got %q", line)
	}
}

func TestLinesAreTruncatedToWidth(t *testing.T) {
	cfg := testConfig()
	long := strings.Repeat("very-long-task-name-", 4)
	cfg.Tasks[long] = config.Task{Description: strings.Repeat("a description that goes on ", 10)}
	m := update(t, loaded(t, cfg), tea.WindowSizeMsg{Width: 60, Height: 20})

	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("line is %d columns wide, want at most 60: %q", w, ansi.Strip(line))
		}
	}
	if !strings.Contains(view(m), "…") {
		t.Error("expected truncated lines to end with …")
	}
}

func TestHelpOverlayListsEveryKey(t *testing.T) {
	m := update(t, loaded(t, testConfig()), tea.WindowSizeMsg{Width: 100, Height: 40})
	m = press(t, m, "?")
	v := view(m)
	for _, k := range keyHelp {
		if !strings.Contains(v, k[0]) || !strings.Contains(v, k[1]) {
			t.Errorf("expected help for %q, got:\n%s", k[0], v)
		}
	}

	if m = press(t, m, "j"); !m.showHelp {
		t.Error("expected other keys to leave help open")
	}
	if m = press(t, m, "esc"); m.showHelp {
		t.Error("expected esc to close help")
	}
	if m = press(t, m, "?", "?"); m.showHelp {
		t.Error("expected ? to toggle help")
	}
}

func TestQuit(t *testing.T) {
	m := loaded(t, testConfig())
	for _, k := range []string{"q", "ctrl+c"} {
		if _, cmd := pressCmd(t, m, k); !isQuit(cmd) {
			t.Errorf("expected %s to quit", k)
		}
	}
	if _, cmd := pressCmd(t, New(testOptions), "ctrl+c"); !isQuit(cmd) {
		t.Error("expected ctrl+c to quit while loading")
	}
}

func TestEnterHandsOff(t *testing.T) {
	m := loaded(t, testConfig())
	if _, cmd := pressCmd(t, m, "enter"); cmd == nil {
		t.Error("expected enter to run the task")
	}
	if _, cmd := pressCmd(t, press(t, m, "tab"), "enter"); cmd == nil {
		t.Error("expected enter to enter the shell")
	}
	if _, cmd := pressCmd(t, loaded(t, &config.Config{}), "enter"); cmd != nil {
		t.Error("expected enter on an empty list to do nothing")
	}
}

func TestHandOffCommandUsesOptions(t *testing.T) {
	m := loaded(t, testConfig())
	cmd := m.command("run", "build", "test")
	if cmd.Path != testOptions.Executable {
		t.Errorf("Path = %q, want %q", cmd.Path, testOptions.Executable)
	}
	want := []string{testOptions.Executable, "-f", "/src/app", "-v", "run", "build", "test"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
}

func TestHandOffPausesAfterTheTask(t *testing.T) {
	var out bytes.Buffer
	h := &handOff{cmd: exec.Command("sh", "-c", "echo task output; exit 3"), pause: true}
	h.SetStdin(strings.NewReader("x"))
	h.SetStdout(&out)
	h.SetStderr(&out)

	err := h.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("expected the task's exit status, got %v", err)
	}
	if !strings.Contains(out.String(), "task output") || !strings.HasSuffix(out.String(), returnPrompt) {
		t.Errorf("expected the output then the prompt, got %q", out.String())
	}
	if h.elapsed <= 0 {
		t.Error("expected the run to be timed")
	}
}

func exitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatal("expected an exit error")
	}
	return err
}

func TestStatusAfterRun(t *testing.T) {
	tests := []struct {
		name string
		msg  runFinishedMsg
		want string
	}{
		{"passed", runFinishedMsg{names: []string{"build"}, elapsed: 53400 * time.Millisecond}, "✓ build passed · 53.4s"},
		{"failed", runFinishedMsg{names: []string{"build"}, err: exitError(t, 1), elapsed: 3100 * time.Millisecond}, "✗ build failed · exit 1 · 3.1s"},
		{"exit code passed through", runFinishedMsg{names: []string{"test"}, err: exitError(t, 2), elapsed: time.Second}, "✗ test failed · exit 2 · 1.0s"},
		{"interrupted", runFinishedMsg{names: []string{"build"}, err: exitError(t, 130)}, "⏹ build interrupted"},
		{"did not start", runFinishedMsg{names: []string{"build"}, err: errors.New("fork/exec: no such file")}, "✗ build failed · fork/exec: no such file"},
		{"several tasks", runFinishedMsg{names: []string{"build", "lint"}, elapsed: 2 * time.Second}, "✓ build, lint passed · 2.0s"},
		{"many tasks", runFinishedMsg{names: []string{"a", "b", "c", "d"}, elapsed: 2 * time.Second}, "✓ 4 tasks passed · 2.0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := update(t, loaded(t, testConfig()), tt.msg)
			if !strings.Contains(view(m), tt.want) {
				t.Errorf("expected %q in the status bar, got:\n%s", tt.want, view(m))
			}
		})
	}
}

func TestStatusAfterShell(t *testing.T) {
	m := update(t, loaded(t, testConfig()), shellFinishedMsg{name: "ci", err: exitError(t, 1)})
	if !strings.Contains(view(m), "left shell ci · exit 1") {
		t.Errorf("expected the shell's exit in the status bar, got:\n%s", view(m))
	}
}
