package ui

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
)

// TaskStatus represents the current lifecycle state of a task
type TaskStatus int

const (
	TaskPending TaskStatus = iota
	TaskRunning
	TaskSuccess
	TaskFailed
	TaskCached
	TaskSkipped
)

// spinnerFrames are the braille spinner characters
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// taskState holds the live state for a single task in the display
type taskState struct {
	mu        sync.Mutex
	name      string
	status    TaskStatus
	startTime time.Time
	duration  time.Duration
	err       error
	lines     *ringBuffer   // last N lines of output
	fullBuf   *bytes.Buffer // complete output for post-run summary
}

// ringBuffer is a fixed-size circular buffer of strings
type ringBuffer struct {
	items []string
	head  int // index of next write position
	count int // number of items stored (up to cap)
	cap   int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		items: make([]string, capacity),
		cap:   capacity,
	}
}

// Push adds a line to the ring buffer, evicting the oldest if full
func (rb *ringBuffer) Push(line string) {
	rb.items[rb.head] = line
	rb.head = (rb.head + 1) % rb.cap
	if rb.count < rb.cap {
		rb.count++
	}
}

// Lines returns the stored lines in chronological order
func (rb *ringBuffer) Lines() []string {
	if rb.count == 0 {
		return nil
	}
	result := make([]string, rb.count)
	start := (rb.head - rb.count + rb.cap) % rb.cap
	for i := 0; i < rb.count; i++ {
		result[i] = rb.items[(start+i)%rb.cap]
	}
	return result
}

// ProgressDisplayOptions configures the progress display
type ProgressDisplayOptions struct {
	TailLines    int           // lines of output per task (default 5)
	TickInterval time.Duration // render tick (default 100ms)
	Out          *os.File      // output file (default os.Stdout)
}

// ProgressDisplay manages the live terminal status region
type ProgressDisplay struct {
	mu           sync.Mutex
	tasks        map[string]*taskState
	order        []string // insertion-ordered task names for stable rendering
	tailLines    int
	spinnerIdx   int
	renderedRows int // number of terminal rows last rendered

	out      *os.File
	done     chan struct{}
	stopped  chan struct{}
	ticker   *time.Ticker
	notifyCh chan struct{} // buffered(1), poked on state change
	stopOnce sync.Once
}

// NewProgressDisplay creates a new progress display
func NewProgressDisplay(opts ProgressDisplayOptions) *ProgressDisplay {
	if opts.TailLines <= 0 {
		opts.TailLines = 5
	}
	if opts.TickInterval <= 0 {
		opts.TickInterval = 100 * time.Millisecond
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	return &ProgressDisplay{
		tasks:     make(map[string]*taskState),
		tailLines: opts.TailLines,
		out:       opts.Out,
		done:      make(chan struct{}),
		stopped:   make(chan struct{}),
		ticker:    time.NewTicker(opts.TickInterval),
		notifyCh:  make(chan struct{}, 1),
	}
}

// RegisterTask creates a taskState entry. Must be called before any writes.
func (pd *ProgressDisplay) RegisterTask(name string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	if _, exists := pd.tasks[name]; exists {
		return
	}
	ts := &taskState{
		name:    name,
		status:  TaskPending,
		lines:   newRingBuffer(pd.tailLines),
		fullBuf: &bytes.Buffer{},
	}
	pd.tasks[name] = ts
	pd.order = append(pd.order, name)
}

// TaskStarted marks a task as running
func (pd *ProgressDisplay) TaskStarted(name string) {
	pd.mu.Lock()
	ts, ok := pd.tasks[name]
	pd.mu.Unlock()
	if !ok {
		return
	}
	ts.mu.Lock()
	ts.status = TaskRunning
	ts.startTime = time.Now()
	ts.mu.Unlock()
	pd.notify()
}

// TaskFinished marks a task as complete
func (pd *ProgressDisplay) TaskFinished(name string, err error, duration time.Duration, cached bool) {
	pd.mu.Lock()
	ts, ok := pd.tasks[name]
	pd.mu.Unlock()
	if !ok {
		return
	}
	ts.mu.Lock()
	if cached {
		ts.status = TaskCached
	} else if err != nil {
		ts.status = TaskFailed
		ts.err = err
	} else {
		ts.status = TaskSuccess
	}
	ts.duration = duration
	ts.mu.Unlock()
	pd.notify()
}

// TaskSkipped marks a task as skipped
func (pd *ProgressDisplay) TaskSkipped(name string) {
	pd.mu.Lock()
	ts, ok := pd.tasks[name]
	pd.mu.Unlock()
	if !ok {
		return
	}
	ts.mu.Lock()
	ts.status = TaskSkipped
	ts.mu.Unlock()
	pd.notify()
}

// GetBuffer returns the full buffered output for a task
func (pd *ProgressDisplay) GetBuffer(name string) string {
	pd.mu.Lock()
	ts, ok := pd.tasks[name]
	pd.mu.Unlock()
	if !ok {
		return ""
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.fullBuf.String()
}

// Start begins the render loop goroutine and hides the cursor
func (pd *ProgressDisplay) Start() {
	fmt.Fprint(pd.out, "\033[?25l")
	go pd.renderLoop()
}

// Stop terminates the render loop, does a final render, and restores the cursor.
// Safe to call multiple times (subsequent calls are no-ops).
func (pd *ProgressDisplay) Stop() {
	pd.stopOnce.Do(func() {
		close(pd.done)
		<-pd.stopped
		pd.ticker.Stop()

		// Final render showing completed states
		pd.render()

		// Show cursor
		fmt.Fprint(pd.out, "\033[?25h")
	})
}

// notify pokes the render loop for an immediate redraw
func (pd *ProgressDisplay) notify() {
	select {
	case pd.notifyCh <- struct{}{}:
	default:
	}
}

func (pd *ProgressDisplay) renderLoop() {
	defer close(pd.stopped)
	for {
		select {
		case <-pd.done:
			return
		case <-pd.ticker.C:
			pd.mu.Lock()
			pd.spinnerIdx = (pd.spinnerIdx + 1) % len(spinnerFrames)
			pd.mu.Unlock()
			pd.render()
		case <-pd.notifyCh:
			pd.render()
		}
	}
}

// render clears the previous status region and redraws all task states.
// All output is assembled in a single buffer and flushed in one write.
func (pd *ProgressDisplay) render() {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	var buf bytes.Buffer

	// Move cursor up to overwrite previous render
	if pd.renderedRows > 0 {
		fmt.Fprintf(&buf, "\033[%dA", pd.renderedRows)
	}

	rows := 0
	for _, name := range pd.order {
		ts := pd.tasks[name]
		ts.mu.Lock()

		switch ts.status {
		case TaskPending:
			ts.mu.Unlock()
			continue

		case TaskRunning:
			spinner := spinnerFrames[pd.spinnerIdx]
			elapsed := time.Since(ts.startTime)
			fmt.Fprintf(&buf, "\033[2K%s %s %s\n",
				Yellow(spinner), name, Gray(FormatDuration(elapsed)))
			rows++

			for _, l := range ts.lines.Lines() {
				truncated := truncateLine(l, pd.termWidth()-4)
				fmt.Fprintf(&buf, "\033[2K    %s\n", Gray(truncated))
				rows++
			}

		case TaskSuccess:
			pd.renderTaskLine(&buf, Green("✓"), name, Gray(FormatDuration(ts.duration)))
			rows++

		case TaskCached:
			pd.renderTaskLine(&buf, Green("✓"), name, Gray("(cached)"))
			rows++

		case TaskFailed:
			pd.renderTaskLine(&buf, Red("✗"), name, Gray(FormatDuration(ts.duration)))
			rows++

		case TaskSkipped:
			pd.renderTaskLine(&buf, Gray("-"), name, Gray("(skipped)"))
			rows++
		}

		ts.mu.Unlock()
	}

	// Clear leftover lines from previous render
	if rows < pd.renderedRows {
		for i := 0; i < pd.renderedRows-rows; i++ {
			fmt.Fprintf(&buf, "\033[2K\n")
		}
		// Move cursor back up to end of new content
		fmt.Fprintf(&buf, "\033[%dA", pd.renderedRows-rows)
	}

	pd.renderedRows = rows

	// Single write to minimize flicker
	pd.out.Write(buf.Bytes())
}

// renderTaskLine writes a formatted task status line to the buffer
func (pd *ProgressDisplay) renderTaskLine(buf *bytes.Buffer, icon, name, suffix string) {
	fmt.Fprintf(buf, "\033[2K%s %s %s\n", icon, name, suffix)
}

// termWidth returns the terminal width, defaulting to 80
func (pd *ProgressDisplay) termWidth() int {
	w, _, err := term.GetSize(pd.out.Fd())
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// truncateLine shortens a string to maxLen, appending "..." if truncated
func truncateLine(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 76
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
