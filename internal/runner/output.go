package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
)

// OutputMode determines how task output is displayed
type OutputMode int

const (
	// Buffered collects output, shows on error (default for local)
	Buffered OutputMode = iota
	// Streaming shows output in real-time with prefixes (default for CI)
	Streaming
)

// DetectOutputMode returns appropriate mode based on environment
func DetectOutputMode() OutputMode {
	if os.Getenv("CI") != "" {
		return Streaming
	}
	return Buffered
}

// OutputManager handles task output based on mode
type OutputManager struct {
	mode    OutputMode
	mu      sync.Mutex
	buffers map[string]*bytes.Buffer
}

// NewOutputManager creates an output manager
func NewOutputManager(mode OutputMode) *OutputManager {
	return &OutputManager{
		mode:    mode,
		buffers: make(map[string]*bytes.Buffer),
	}
}

// Writer returns an io.Writer for task output
func (m *OutputManager) Writer(taskName string) io.Writer {
	switch m.mode {
	case Streaming:
		return &prefixWriter{
			prefix: fmt.Sprintf("[%s] ", taskName),
			out:    os.Stdout,
			mu:     &m.mu,
		}
	case Buffered:
		buf := &bytes.Buffer{}
		m.mu.Lock()
		m.buffers[taskName] = buf
		m.mu.Unlock()
		return buf
	default:
		return io.Discard
	}
}

// FlushOnError prints buffered output for a failed task
func (m *OutputManager) FlushOnError(taskName string) {
	if m.mode != Buffered {
		return
	}

	m.mu.Lock()
	buf, ok := m.buffers[taskName]
	m.mu.Unlock()

	if ok && buf.Len() > 0 {
		fmt.Print(buf.String())
	}
}

// prefixWriter adds a prefix to each line
type prefixWriter struct {
	prefix string
	out    io.Writer
	mu     *sync.Mutex
}

func (w *prefixWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines := bytes.Split(p, []byte("\n"))
	for i, line := range lines {
		if len(line) > 0 || i < len(lines)-1 {
			_, _ = fmt.Fprintf(w.out, "%s%s\n", w.prefix, line)
		}
	}
	return len(p), nil
}
