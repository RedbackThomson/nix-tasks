package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTailWriter(t *testing.T) {
	tests := []struct {
		name   string
		writes []string
		n      int
		want   []string
	}{
		{"splits lines", []string{"line1\nline2\n"}, 5, []string{"line1", "line2"}},
		{"joins partial writes", []string{"partial", " continued\nline2\n"}, 5, []string{"partial continued", "line2"}},
		{"keeps last n", []string{"a\nb\nc\nd\n"}, 2, []string{"c", "d"}},
		{"carriage return replaces line", []string{"50%\r100%\n"}, 5, []string{"100%"}},
		{"shows in-progress line", []string{"done\n", "10%\r", "20%"}, 5, []string{"done", "20%"}},
		{"in-progress line counts toward n", []string{"a\nb\n", "c"}, 2, []string{"b", "c"}},
		{"strips colour", []string{"\x1b[31mred\x1b[0m text\n"}, 5, []string{"red text"}},
		{"strips cursor movement", []string{"\x1b[2K\x1b[1Gtidy\n"}, 5, []string{"tidy"}},
		{"expands tabs and drops control chars", []string{"a\tb\x07\n"}, 5, []string{"a    b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tw := newTailWriter(tt.n, nil)
			for _, w := range tt.writes {
				_, _ = tw.Write([]byte(w))
			}
			if got := tw.tail(tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("tail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTailWriter_UnboundedPartialIsCapped(t *testing.T) {
	tw := newTailWriter(3, nil)
	for range 1000 {
		_, _ = tw.Write([]byte(strings.Repeat("#", 1000)))
	}
	if n := len(tw.lines.partial()); n > maxLineBytes {
		t.Errorf("partial line grew to %d bytes", n)
	}
}

func TestTruncateWidth(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"short", "short", 80, "short"},
		{"long", "this is a very long line", 10, "this is a…"},
		{"exact", "exact", 5, "exact"},
		{"zero width", "abc", 0, ""},
		{"wide runes by column", "界界界界界", 5, "界界…"},
		{"keeps escape sequences whole", "\x1b[31mabcdef\x1b[0m", 4, "\x1b[31mabc…\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateWidth(tt.in, tt.width)
			if got != tt.want {
				t.Errorf("truncateWidth(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
			if w := ansi.StringWidth(got); w > tt.width {
				t.Errorf("width %d exceeds %d", w, tt.width)
			}
		})
	}
}

func TestRingBuffer(t *testing.T) {
	rb := newRingBuffer(2)
	if rb.Lines() != nil {
		t.Error("empty buffer should have no lines")
	}
	rb.Push("a")
	rb.Push("b")
	rb.Push("c")
	if got := rb.Lines(); !slices.Equal(got, []string{"b", "c"}) {
		t.Errorf("Lines() = %v, want [b c]", got)
	}
}
