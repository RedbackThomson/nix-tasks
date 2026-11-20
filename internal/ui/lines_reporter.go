package ui

import (
	"io"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// LineReporter prints one line when each task starts and one when it
// finishes. With Stream set, task output is printed as it arrives, each line
// prefixed with the task's display label; otherwise it is held and only failed tasks'
// output is shown, in the summary. It suits CI logs and piped output, where
// the terminal cannot be redrawn.
type LineReporter struct {
	out    io.Writer
	stream bool

	mu      sync.Mutex // serializes whole-line writes to out
	writers map[string]*prefixWriter
	outputs outputBuffers
	labels  map[string]string
}

// NewLineReporter creates a LineReporter writing to out.
func NewLineReporter(out io.Writer, stream bool) *LineReporter {
	return &LineReporter{out: out, stream: stream, writers: map[string]*prefixWriter{}}
}

func (r *LineReporter) Start()     {}
func (r *LineReporter) Interrupt() {}
func (r *LineReporter) Stop()      {}

func (r *LineReporter) RunPlanned(plan runner.RunPlan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.labels = plan.Labels
}

func (r *LineReporter) TaskStarted(name string) {
	if r.stream {
		r.writeLine(formatStarted(withLabel(name, r.label(name))))
		return
	}
	r.writeLine(formatStarted(name))
}

func (r *LineReporter) label(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return labelOf(r.labels, name)
}

func (r *LineReporter) TaskOutput(name string) (io.Writer, io.Writer) {
	if !r.stream {
		w := r.outputs.writer(name)
		return w, w
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.writers[name]
	if !ok {
		w = &prefixWriter{prefix: "[" + labelOf(r.labels, name) + "] ", out: r.out, outMu: &r.mu}
		r.writers[name] = w
	}
	return w, w
}

func (r *LineReporter) TaskFinished(res runner.TaskResult) {
	r.mu.Lock()
	w := r.writers[res.Name]
	delete(r.writers, res.Name)
	r.mu.Unlock()
	if w != nil {
		w.Close()
	}
	if !failed(res) {
		r.outputs.drop(res.Name)
	}
	r.writeLine(FormatResult(res))
}

func (r *LineReporter) Summary(graph *runner.TaskGraph, targets []string, results []runner.TaskResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	writeSummary(r.out, graph, targets, results, func(name string) ([]byte, bool) {
		if r.stream {
			return nil, false
		}
		return r.outputs.get(name)
	})
}

func (r *LineReporter) writeLine(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = io.WriteString(r.out, line+"\n")
}

// prefixWriter prefixes each line of a task's output. It holds a partial
// line until its newline arrives (or Close), and writes each line with a
// single Write under outMu so lines from parallel tasks never interleave.
type prefixWriter struct {
	prefix string
	out    io.Writer
	outMu  *sync.Mutex

	mu    sync.Mutex
	lines lineSplitter
	buf   []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines.feed(p, w.emit)
	return len(p), nil
}

// Close writes any unterminated final line.
func (w *prefixWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines.flush(w.emit)
	return nil
}

func (w *prefixWriter) emit(line []byte) {
	w.buf = append(w.buf[:0], w.prefix...)
	w.buf = append(w.buf, line...)
	w.buf = append(w.buf, '\n')
	w.outMu.Lock()
	defer w.outMu.Unlock()
	_, _ = w.out.Write(w.buf)
}
