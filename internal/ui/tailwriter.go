package ui

import (
	"bytes"
	"io"
	"sync"
)

// tailWriter is an io.Writer that splits incoming bytes into lines,
// pushes complete lines into the task's ring buffer, and also writes
// everything to the full buffer (for post-run error display).
type tailWriter struct {
	state   *taskState
	display *ProgressDisplay

	mu      sync.Mutex
	partial bytes.Buffer // accumulates bytes until a newline
}

// Writer returns a thread-safe io.Writer that feeds into the task's
// ring buffer and full buffer simultaneously.
func (pd *ProgressDisplay) Writer(name string) io.Writer {
	pd.mu.Lock()
	ts, ok := pd.tasks[name]
	pd.mu.Unlock()
	if !ok {
		return io.Discard
	}
	return &tailWriter{
		state:   ts,
		display: pd,
	}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	n := len(p)

	// Always write to full buffer (unmodified)
	tw.state.mu.Lock()
	tw.state.fullBuf.Write(p)
	tw.state.mu.Unlock()

	// Strip \r before line-splitting for the ring buffer.
	// Tools like `go test` emit \r for in-place progress updates;
	// these corrupt the progress display's ANSI cursor positioning.
	cleaned := bytes.ReplaceAll(p, []byte("\r"), nil)

	// Split into lines for the ring buffer
	remaining := cleaned
	for len(remaining) > 0 {
		idx := bytes.IndexByte(remaining, '\n')
		if idx == -1 {
			// No newline -- accumulate partial line
			tw.partial.Write(remaining)
			break
		}

		// Complete a line: partial + everything before \n
		tw.partial.Write(remaining[:idx])
		line := tw.partial.String()
		tw.partial.Reset()

		// Push into ring buffer
		tw.state.mu.Lock()
		tw.state.lines.Push(line)
		tw.state.mu.Unlock()

		remaining = remaining[idx+1:]
	}

	// Notify the display that content changed
	tw.display.notify()

	return n, nil
}
