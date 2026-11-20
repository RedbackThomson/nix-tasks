package ui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// tailWriter keeps the last few lines of a task's output for the live
// progress display. Lines are sanitized so they can be drawn on one terminal
// row: escape sequences and control characters are removed.
type tailWriter struct {
	mu     sync.Mutex
	lines  lineSplitter
	ring   *ringBuffer
	notify func()
}

func newTailWriter(n int, notify func()) *tailWriter {
	return &tailWriter{ring: newRingBuffer(n), notify: notify}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	tw.lines.feed(p, func(line []byte) {
		tw.ring.Push(sanitizeLine(string(line)))
	})
	tw.mu.Unlock()
	if tw.notify != nil {
		tw.notify()
	}
	return len(p), nil
}

// tail returns up to the last n lines, including a line still being written
// so that \r-driven progress shows up as it happens.
func (tw *tailWriter) tail(n int) []string {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	lines := tw.ring.Lines()
	if p := tw.lines.partial(); len(p) > 0 {
		lines = append(lines, sanitizeLine(string(p)))
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// sanitizeLine removes escape sequences and control characters, and expands
// tabs, so the line's display width is what it looks like.
func sanitizeLine(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\t", "    "))
}

// truncateWidth shortens s to at most width terminal columns, ending in an
// ellipsis when cut. It never splits a rune or an escape sequence.
func truncateWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
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
