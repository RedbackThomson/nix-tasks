package ui

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func feedAll(writes ...string) (lines []string, partial string) {
	var s lineSplitter
	emit := func(l []byte) { lines = append(lines, string(l)) }
	for _, w := range writes {
		s.feed([]byte(w), emit)
	}
	return lines, string(s.partial())
}

func TestLineSplitter(t *testing.T) {
	tests := []struct {
		name        string
		writes      []string
		wantLines   []string
		wantPartial string
	}{
		{"whole lines", []string{"a\nb\n"}, []string{"a", "b"}, ""},
		{"line split across writes", []string{"Downloading...", " done\n"}, []string{"Downloading... done"}, ""},
		{"partial held", []string{"a\nb"}, []string{"a"}, "b"},
		{"blank line kept", []string{"a\n\nb\n"}, []string{"a", "", "b"}, ""},
		{"carriage return replaces line", []string{"10%\r50%\r100%\n"}, []string{"100%"}, ""},
		{"crlf ends a line", []string{"a\r\nb\r\n"}, []string{"a", "b"}, ""},
		{"crlf split across writes", []string{"a\r", "\nb\n"}, []string{"a", "b"}, ""},
		{"pending carriage return keeps last text", []string{"50%\r"}, nil, "50%"},
		{"carriage return then new text", []string{"50%\r", "60%"}, nil, "60%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, partial := feedAll(tt.writes...)
			if !slices.Equal(lines, tt.wantLines) {
				t.Errorf("lines = %q, want %q", lines, tt.wantLines)
			}
			if partial != tt.wantPartial {
				t.Errorf("partial = %q, want %q", partial, tt.wantPartial)
			}
		})
	}
}

func TestLineSplitter_CapsUnterminatedLine(t *testing.T) {
	var s lineSplitter
	var lines []string
	emit := func(l []byte) { lines = append(lines, string(l)) }
	// A wide rune straddles the cap, so the break must back up to its start.
	chunk := strings.Repeat("x", maxLineBytes-1) + "界"
	for range 10 {
		s.feed([]byte(chunk), emit)
	}
	if len(s.partial()) > maxLineBytes {
		t.Errorf("partial grew to %d bytes", len(s.partial()))
	}
	if len(lines) == 0 {
		t.Fatal("expected long output to be broken into lines")
	}
	for _, l := range lines {
		if len(l) > maxLineBytes || !utf8.ValidString(l) {
			t.Fatalf("bad line: %d bytes, valid=%v", len(l), utf8.ValidString(l))
		}
	}
}

func TestLineSplitter_CarriageReturnProgressStaysBounded(t *testing.T) {
	var s lineSplitter
	for i := range 100000 {
		s.feed([]byte(fmt.Sprintf("%d%%\r", i%100)), func([]byte) { t.Fatal("no line expected") })
	}
	if got := string(s.partial()); got != "99%" {
		t.Errorf("partial = %q, want last progress update", got)
	}
}

func TestPrefixWriter(t *testing.T) {
	tests := []struct {
		name   string
		writes []string
		want   string
	}{
		{"single line", []string{"hello\n"}, "[t] hello\n"},
		{"multiple lines", []string{"a\nb\n"}, "[t] a\n[t] b\n"},
		{"split write is one line", []string{"Downloading...", " done\n"}, "[t] Downloading... done\n"},
		{"unterminated line flushed on close", []string{"a"}, "[t] a\n"},
		{"blank line kept", []string{"a\n\nb\n"}, "[t] a\n[t] \n[t] b\n"},
		{"carriage return replaces line", []string{"10%\r50%\r100%\n"}, "[t] 100%\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := &prefixWriter{prefix: "[t] ", out: &out, outMu: &sync.Mutex{}}
			for _, s := range tt.writes {
				n, err := w.Write([]byte(s))
				if err != nil || n != len(s) {
					t.Fatalf("Write() = %d, %v", n, err)
				}
			}
			_ = w.Close()
			if got := out.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

// Large chunks split lines arbitrarily, and parallel tasks write at once;
// every output line must still be one whole input line.
func TestPrefixWriter_ParallelTasksNeverInterleave(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup
	for task := range 4 {
		w := &prefixWriter{prefix: fmt.Sprintf("[%d] ", task), out: &out, outMu: &mu}
		var input strings.Builder
		for i := range 500 {
			fmt.Fprintf(&input, "task %d line %d %s\n", task, i, strings.Repeat("y", i%70))
		}
		data := []byte(input.String())
		wg.Add(1)
		go func() {
			defer wg.Done()
			for len(data) > 0 {
				n := min(len(data), 37)
				_, _ = w.Write(data[:n])
				data = data[n:]
			}
			_ = w.Close()
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2000 {
		t.Fatalf("got %d lines, want 2000", len(lines))
	}
	whole := regexp.MustCompile(`^\[(\d)\] task (\d) line (\d+) (y*)$`)
	for _, l := range lines {
		m := whole.FindStringSubmatch(l)
		if m == nil || m[1] != m[2] {
			t.Fatalf("mangled line %q", l)
		}
		if i, _ := strconv.Atoi(m[3]); len(m[4]) != i%70 {
			t.Fatalf("mangled line %q", l)
		}
	}
}
