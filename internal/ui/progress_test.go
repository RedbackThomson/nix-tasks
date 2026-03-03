package ui

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestRingBuffer_Basic(t *testing.T) {
	rb := newRingBuffer(3)
	rb.Push("a")
	rb.Push("b")
	rb.Push("c")

	lines := rb.Lines()
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Errorf("lines = %v, want [a b c]", lines)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := newRingBuffer(2)
	rb.Push("a")
	rb.Push("b")
	rb.Push("c") // evicts "a"

	lines := rb.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "b" || lines[1] != "c" {
		t.Errorf("lines = %v, want [b c]", lines)
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := newRingBuffer(5)
	lines := rb.Lines()
	if lines != nil {
		t.Errorf("expected nil, got %v", lines)
	}
}

func TestRingBuffer_SingleCapacity(t *testing.T) {
	rb := newRingBuffer(1)
	rb.Push("a")
	rb.Push("b")

	lines := rb.Lines()
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0] != "b" {
		t.Errorf("lines = %v, want [b]", lines)
	}
}

func TestRingBuffer_PartialFill(t *testing.T) {
	rb := newRingBuffer(5)
	rb.Push("a")
	rb.Push("b")

	lines := rb.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" {
		t.Errorf("lines = %v, want [a b]", lines)
	}
}

func TestTailWriter_SplitsLines(t *testing.T) {
	ts := &taskState{
		name:    "test",
		lines:   newRingBuffer(10),
		fullBuf: &bytes.Buffer{},
	}
	pd := &ProgressDisplay{
		tasks:    map[string]*taskState{"test": ts},
		notifyCh: make(chan struct{}, 1),
	}
	tw := &tailWriter{state: ts, display: pd}

	tw.Write([]byte("line1\nline2\n"))

	lines := ts.lines.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("lines = %v, want [line1 line2]", lines)
	}
}

func TestTailWriter_PartialLines(t *testing.T) {
	ts := &taskState{
		name:    "test",
		lines:   newRingBuffer(10),
		fullBuf: &bytes.Buffer{},
	}
	pd := &ProgressDisplay{
		tasks:    map[string]*taskState{"test": ts},
		notifyCh: make(chan struct{}, 1),
	}
	tw := &tailWriter{state: ts, display: pd}

	tw.Write([]byte("partial"))
	tw.Write([]byte(" continued\nline2\n"))

	lines := ts.lines.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "partial continued" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "partial continued")
	}
	if lines[1] != "line2" {
		t.Errorf("lines[1] = %q, want %q", lines[1], "line2")
	}
}

func TestTailWriter_FullBuffer(t *testing.T) {
	ts := &taskState{
		name:    "test",
		lines:   newRingBuffer(3),
		fullBuf: &bytes.Buffer{},
	}
	pd := &ProgressDisplay{
		tasks:    map[string]*taskState{"test": ts},
		notifyCh: make(chan struct{}, 1),
	}
	tw := &tailWriter{state: ts, display: pd}

	tw.Write([]byte("line1\nline2\nline3\n"))

	// Full buffer should have everything
	want := "line1\nline2\nline3\n"
	if ts.fullBuf.String() != want {
		t.Errorf("fullBuf = %q, want %q", ts.fullBuf.String(), want)
	}
}

func TestTailWriter_ConcurrentWrites(t *testing.T) {
	ts := &taskState{
		name:    "test",
		lines:   newRingBuffer(100),
		fullBuf: &bytes.Buffer{},
	}
	pd := &ProgressDisplay{
		tasks:    map[string]*taskState{"test": ts},
		notifyCh: make(chan struct{}, 1),
	}
	tw := &tailWriter{state: ts, display: pd}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tw.Write([]byte(fmt.Sprintf("goroutine %d line %d\n", n, j)))
			}
		}(i)
	}
	wg.Wait()

	// Full buffer should have all 1000 lines
	fullLines := strings.Count(ts.fullBuf.String(), "\n")
	if fullLines != 1000 {
		t.Errorf("expected 1000 lines in full buffer, got %d", fullLines)
	}
}

func TestTruncateLine(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 80, "short"},
		{"this is a very long line", 20, "this is a very lo..."},
		{"exact", 5, "exact"},
		{"ab", 1, "a"},
		{"abcdef", 3, "abc"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q/%d", tt.input, tt.maxLen), func(t *testing.T) {
			got := truncateLine(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateLine(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateLine_DefaultMaxLen(t *testing.T) {
	short := "hello"
	got := truncateLine(short, 0)
	if got != short {
		t.Errorf("truncateLine(%q, 0) = %q, want %q", short, got, short)
	}
}
