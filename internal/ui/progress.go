package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

const (
	hideCursor = "\033[?25l"
	showCursor = "\033[?25h"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ProgressOptions configures a ProgressReporter.
type ProgressOptions struct {
	Out          io.Writer
	TailLines    int           // output lines shown per running task (default 5)
	TickInterval time.Duration // spinner redraw interval (default 100ms)
	// Size returns the terminal's width and height. It defaults to the size
	// of Out when Out is a terminal, else 80x24.
	Size func() (width, height int)
	Now  func() time.Time
}

// ProgressReporter draws a live region at the bottom of the terminal showing
// each running task with the tail of its output. Each finished task is
// printed once, above the region, so it scrolls away like normal output and
// the region only ever holds what is still running.
type ProgressReporter struct {
	opts ProgressOptions

	mu           sync.Mutex
	running      []*liveTask
	pending      []string // finished lines not yet printed
	renderedRows int
	spinnerIdx   int
	started      bool
	interrupted  bool

	outputs  outputBuffers
	notifyCh chan struct{}
	done     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
}

type liveTask struct {
	name  string
	start time.Time
	tail  *tailWriter
}

// NewProgressReporter creates a ProgressReporter.
func NewProgressReporter(opts ProgressOptions) *ProgressReporter {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.TailLines <= 0 {
		opts.TailLines = 5
	}
	if opts.TickInterval <= 0 {
		opts.TickInterval = 100 * time.Millisecond
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Size == nil {
		opts.Size = terminalSize(opts.Out)
	}
	return &ProgressReporter{
		opts:     opts,
		notifyCh: make(chan struct{}, 1),
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func terminalSize(out io.Writer) func() (int, int) {
	return func() (int, int) {
		if f, ok := out.(*os.File); ok {
			if w, h, err := term.GetSize(f.Fd()); err == nil && w > 0 && h > 0 {
				return w, h
			}
		}
		return 80, 24
	}
}

// Start hides the cursor and begins redrawing the live region.
func (p *ProgressReporter) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	if !p.interrupted {
		_, _ = io.WriteString(p.opts.Out, hideCursor)
	}
	go p.loop()
}

// Interrupt shows the cursor again immediately, so a second Ctrl-C that
// kills the process outright cannot leave it hidden.
func (p *ProgressReporter) Interrupt() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interrupted = true
	_, _ = io.WriteString(p.opts.Out, showCursor)
}

// Stop clears the live region, prints any remaining finished lines and
// restores the cursor.
func (p *ProgressReporter) Stop() {
	p.stopOnce.Do(func() {
		close(p.done)
		p.mu.Lock()
		started := p.started
		p.mu.Unlock()
		if started {
			<-p.stopped
		}
		p.render()
		if started {
			_, _ = io.WriteString(p.opts.Out, showCursor)
		}
	})
}

func (p *ProgressReporter) Summary(graph *runner.TaskGraph, targets []string, results []runner.TaskResult) {
	p.Stop()
	p.mu.Lock()
	defer p.mu.Unlock()
	writeSummary(p.opts.Out, graph, targets, results, p.outputs.get)
}

func (p *ProgressReporter) TaskStarted(name string) {
	p.mu.Lock()
	p.running = append(p.running, &liveTask{
		name:  name,
		start: p.opts.Now(),
		tail:  newTailWriter(p.opts.TailLines, p.notify),
	})
	p.mu.Unlock()
	p.notify()
}

func (p *ProgressReporter) TaskOutput(name string) (io.Writer, io.Writer) {
	full := p.outputs.writer(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.running {
		if t.name == name {
			w := io.MultiWriter(full, t.tail)
			return w, w
		}
	}
	return full, full
}

func (p *ProgressReporter) TaskFinished(r runner.TaskResult) {
	p.mu.Lock()
	p.running = slices.DeleteFunc(p.running, func(t *liveTask) bool { return t.name == r.Name })
	p.pending = append(p.pending, FormatResult(r))
	p.mu.Unlock()
	if !failed(r) {
		p.outputs.drop(r.Name)
	}
	p.render()
}

func (p *ProgressReporter) notify() {
	select {
	case p.notifyCh <- struct{}{}:
	default:
	}
}

func (p *ProgressReporter) loop() {
	defer close(p.stopped)
	ticker := time.NewTicker(p.opts.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.mu.Lock()
			p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
			p.mu.Unlock()
			p.render()
		case <-p.notifyCh:
			p.render()
		}
	}
}

// render replaces the previous live region with any newly finished lines
// followed by the current region, in a single write.
func (p *ProgressReporter) render() {
	p.mu.Lock()
	defer p.mu.Unlock()

	var buf bytes.Buffer
	if p.renderedRows > 0 {
		fmt.Fprintf(&buf, "\033[%dA", p.renderedRows)
	}
	if p.renderedRows > 0 || len(p.pending) > 0 {
		buf.WriteString("\r\033[J")
	}
	for _, line := range p.pending {
		buf.WriteString(line + "\n")
	}
	p.pending = p.pending[:0]

	rows := p.regionRows()
	for _, row := range rows {
		buf.WriteString(row + "\n")
	}
	p.renderedRows = len(rows)

	if buf.Len() > 0 {
		_, _ = p.opts.Out.Write(buf.Bytes())
	}
}

// regionRows lays out the live region. Every row fits within the terminal
// width so the row count is exact, and the region never grows taller than the
// terminal, since rows that scroll off can no longer be redrawn.
func (p *ProgressReporter) regionRows() []string {
	if len(p.running) == 0 {
		return nil
	}
	width, height := p.opts.Size()
	width-- // writing the last column makes some terminals wrap early
	maxRows := max(height-1, 1)

	tasks := p.running
	var hidden int
	if len(tasks) > maxRows {
		keep := max(maxRows-1, 0)
		hidden = len(tasks) - keep
		tasks = tasks[:keep]
	}
	perTask := 0
	if len(tasks) > 0 && hidden == 0 {
		perTask = min(p.opts.TailLines, (maxRows-len(tasks))/len(tasks))
	}

	now := p.opts.Now()
	spinner := spinnerFrames[p.spinnerIdx]
	var rows []string
	for _, t := range tasks {
		elapsed := FormatDuration(now.Sub(t.start))
		name := truncateWidth(t.name, width-4-len(elapsed))
		rows = append(rows, fmt.Sprintf("%s %s %s", Accent(spinner), name, Muted(elapsed)))
		if perTask == 0 {
			continue
		}
		for _, line := range t.tail.tail(perTask) {
			rows = append(rows, "    "+Muted(truncateWidth(line, width-4)))
		}
	}
	if hidden > 0 {
		rows = append(rows, Muted(truncateWidth(fmt.Sprintf("... %d more running", hidden), width)))
	}
	return rows
}
