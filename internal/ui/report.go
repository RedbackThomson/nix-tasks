package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// FailedOutputTailLines is how many lines of a failed task's output are shown.
const FailedOutputTailLines = 50

// Reporter renders a run: it receives every task event from the runner and
// owns everything written to the terminal while tasks run, so it is the only
// thing that decides where lines start and end.
type Reporter interface {
	runner.Reporter
	// Start begins live output.
	Start()
	// Interrupt is called from the signal handler on the first interrupt,
	// before a second one can kill the process, so terminal state is
	// restored even if cleanup never runs.
	Interrupt()
	// Stop ends live output. It is safe to call more than once.
	Stop()
	// Summary stops live output and prints the end-of-run summary: the
	// dependency tree of targets in graph with each task's result.
	Summary(graph *runner.TaskGraph, targets []string, results []runner.TaskResult)
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("(%dms)", d.Milliseconds())
	}
	return fmt.Sprintf("(%.1fs)", d.Seconds())
}

// StateIcon returns the coloured marker for a task state.
func StateIcon(state runner.TaskState) string {
	switch state {
	case runner.Failed:
		return Failure("✗")
	case runner.FailedAllowed:
		return Warning("!")
	case runner.Skipped:
		return Muted("-")
	case runner.Interrupted:
		return Warning("⏹")
	}
	return Success("✓")
}

// FormatResult is the one-line status every reporter prints for a finished
// task, e.g. "✓ build (2.3s)" or "✗ test (4.1s) (exit code 2)".
func FormatResult(r runner.TaskResult) string {
	label := StateIcon(r.State) + " " + r.Name + resultDetail(r)
	return label
}

// resultDetail is what follows a task's name and marker: how long it took or
// that it was cached, and why it did not pass.
func resultDetail(r runner.TaskResult) string {
	var detail string
	switch {
	case r.Cached && r.Saved > 0:
		detail += " " + Muted("(cached, "+ShortDuration(r.Saved)+" saved)")
	case r.Cached:
		detail += " " + Muted("(cached)")
	case r.State != runner.Skipped:
		detail += " " + Muted(FormatDuration(r.Duration))
	}
	switch r.State {
	case runner.Failed:
		if code := r.ExitCode(); code > 0 {
			detail += " " + Muted(fmt.Sprintf("(exit code %d)", code))
		}
	case runner.FailedAllowed:
		detail += " " + Muted("(failure allowed)")
	case runner.Skipped:
		detail += " " + Muted("(skipped)")
	case runner.Interrupted:
		detail += " " + Muted("(interrupted)")
	}
	return detail
}

// ShortDuration formats a duration without parentheses, to the precision
// worth reading: "350ms", "4.1s", "41s", "2m5s".
func ShortDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func formatWarning(task, warning string) string {
	return Warning("warning:") + " " + task + ": " + warning
}

// labelOf returns name's display label, or name when it has none.
func labelOf(labels map[string]string, name string) string {
	if l := labels[name]; l != "" {
		return l
	}
	return name
}

// withLabel names a task by its label and full name when they differ, so
// prefixed output can be traced back to the task.
func withLabel(name, label string) string {
	if label == name {
		return name
	}
	return label + " = " + name
}

// formatStarted is the line printed when a task starts.
func formatStarted(name string) string {
	return Accent("▶") + " " + name
}

func failed(r runner.TaskResult) bool {
	return r.State == runner.Failed || r.State == runner.FailedAllowed
}

// errorDetail returns the error to show for a failed task, or "" when its
// output and exit code already explain it.
func errorDetail(r runner.TaskResult) string {
	if r.Error == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(r.Error, &exitErr) {
		return ""
	}
	return r.Error.Error()
}

// writeFailure writes a failed task's error and the tail of its output,
// indented. When a real failure's output is longer than the tail, the full
// output is saved to a temp file whose path is printed; an allowed failure
// does not fail the run, so it leaves no files behind.
func writeFailure(b *strings.Builder, r runner.TaskResult, output []byte) {
	if detail := errorDetail(r); detail != "" {
		fmt.Fprintf(b, "    %s\n", Failure("error: "+detail))
	}
	lines := splitLines(output)
	omitted := 0
	if len(lines) > FailedOutputTailLines {
		omitted = len(lines) - FailedOutputTailLines
		lines = lines[omitted:]
	}
	if omitted > 0 {
		fmt.Fprintf(b, "    %s\n", Muted(fmt.Sprintf("... %d lines omitted ...", omitted)))
	}
	for _, line := range lines {
		if !ColorEnabled() {
			line = ansi.Strip(line)
		}
		fmt.Fprintf(b, "    %s\n", line)
	}
	if omitted > 0 && r.State == runner.Failed {
		if path, err := saveLog(r.Name, output); err == nil {
			fmt.Fprintf(b, "    %s\n", Muted("full output: "+path))
		}
	}
}

func saveLog(name string, output []byte) (string, error) {
	f, err := os.CreateTemp("", "nix-tasks-"+strings.ReplaceAll(name, "/", "_")+"-*.log")
	if err != nil {
		return "", err
	}
	_, err = f.Write(output)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// outputBuffers keeps each task's complete output for the summary.
type outputBuffers struct {
	mu   sync.Mutex
	bufs map[string]*bytes.Buffer
}

func (o *outputBuffers) writer(name string) io.Writer {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.bufs == nil {
		o.bufs = map[string]*bytes.Buffer{}
	}
	if o.bufs[name] == nil {
		o.bufs[name] = &bytes.Buffer{}
	}
	return &bufferWriter{mu: &o.mu, buf: o.bufs[name]}
}

// drop discards a task's output once it can no longer be needed.
func (o *outputBuffers) drop(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.bufs, name)
}

func (o *outputBuffers) get(name string) ([]byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.bufs[name]
	if !ok {
		return nil, true
	}
	return bytes.Clone(b.Bytes()), true
}

type bufferWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
