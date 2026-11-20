package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// returnPrompt is printed after a task run so its output stays readable until
// the user is ready to go back to the TUI.
const returnPrompt = "Press any key to return to nix-tasks…"

// exitInterrupted is what nix-tasks run exits with after Ctrl-C.
const exitInterrupted = 130

// handOff runs a nix-tasks command in the terminal the TUI releases while it
// runs, optionally waiting for a keypress before the TUI takes it back.
type handOff struct {
	cmd     *exec.Cmd
	pause   bool
	stdin   io.Reader
	stdout  io.Writer
	elapsed time.Duration
}

func (h *handOff) SetStdin(r io.Reader) {
	h.stdin = r
	h.cmd.Stdin = r
}

func (h *handOff) SetStdout(w io.Writer) {
	h.stdout = w
	h.cmd.Stdout = w
}

func (h *handOff) SetStderr(w io.Writer) {
	h.cmd.Stderr = w
}

func (h *handOff) Run() error {
	start := time.Now()
	err := h.cmd.Run()
	h.elapsed = time.Since(start)
	if h.pause {
		h.waitForKey()
	}
	return err
}

func (h *handOff) waitForKey() {
	_, _ = fmt.Fprintf(h.stdout, "\n%s", returnPrompt)
	if f, ok := h.stdin.(*os.File); ok && term.IsTerminal(f.Fd()) {
		if state, err := term.MakeRaw(f.Fd()); err == nil {
			defer func() { _ = term.Restore(f.Fd(), state) }()
		}
	}
	var b [1]byte
	_, _ = h.stdin.Read(b[:])
}

// command returns the nix-tasks invocation for args, forwarding the global
// flags the TUI was started with.
func (m Model) command(args ...string) *exec.Cmd {
	argv := append(append([]string{}, m.opts.Args...), args...)
	return exec.Command(m.opts.Executable, argv...)
}

type runFinishedMsg struct {
	names   []string
	err     error
	elapsed time.Duration
}

type shellFinishedMsg struct {
	name string
	err  error
}

func (m Model) runTasks(names []string) tea.Cmd {
	h := &handOff{cmd: m.command(append([]string{"run"}, names...)...), pause: true}
	return tea.Exec(h, func(err error) tea.Msg {
		return runFinishedMsg{names: names, err: err, elapsed: h.elapsed}
	})
}

func (m Model) enterShell(name string) tea.Cmd {
	h := &handOff{cmd: m.command("shell", name)}
	return tea.Exec(h, func(err error) tea.Msg {
		return shellFinishedMsg{name: name, err: err}
	})
}

// runStatus describes how a run ended, from the exit status of nix-tasks run.
func runStatus(msg runFinishedMsg) string {
	label := taskLabel(msg.names)
	took := strings.Trim(ui.FormatDuration(msg.elapsed), "()")
	var exitErr *exec.ExitError
	switch {
	case msg.err == nil:
		return fmt.Sprintf("%s %s passed · %s", successStyle.Render("✓"), label, took)
	case errors.As(msg.err, &exitErr) && (exitErr.ExitCode() == exitInterrupted || exitErr.ExitCode() < 0):
		return fmt.Sprintf("%s %s interrupted", warnStyle.Render("⏹"), label)
	case errors.As(msg.err, &exitErr):
		return fmt.Sprintf("%s %s failed · exit %d · %s", failStyle.Render("✗"), label, exitErr.ExitCode(), took)
	}
	return fmt.Sprintf("%s %s failed · %v", failStyle.Render("✗"), label, msg.err)
}

func shellStatus(msg shellFinishedMsg) string {
	var exitErr *exec.ExitError
	switch {
	case msg.err == nil:
		return fmt.Sprintf("left shell %s", msg.name)
	case errors.As(msg.err, &exitErr):
		return fmt.Sprintf("left shell %s · exit %d", msg.name, exitErr.ExitCode())
	}
	return fmt.Sprintf("%s shell %s failed · %v", failStyle.Render("✗"), msg.name, msg.err)
}

func taskLabel(names []string) string {
	if len(names) > 3 {
		return fmt.Sprintf("%d tasks", len(names))
	}
	return strings.Join(names, ", ")
}
