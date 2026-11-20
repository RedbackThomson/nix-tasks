package ui

import (
	"bytes"
	"unicode/utf8"
)

// maxLineBytes caps how much of an unterminated line is held. Output that
// only ever redraws with \r (curl, docker pull) never sends a newline.
const maxLineBytes = 4096

// lineSplitter turns a byte stream into whole lines. A \r that is not part of
// \r\n starts the current line over, the way a terminal would show it. A line
// longer than maxLineBytes is broken on a rune boundary.
type lineSplitter struct {
	cur []byte
	cr  bool
}

// feed consumes p and calls emit with each completed line, without its line
// terminator. emit must not retain line.
func (s *lineSplitter) feed(p []byte, emit func(line []byte)) {
	for len(p) > 0 {
		if s.cr {
			s.cr = false
			if p[0] != '\n' {
				s.cur = s.cur[:0]
			}
		}
		i := bytes.IndexAny(p, "\r\n")
		if i < 0 {
			s.append(p, emit)
			return
		}
		s.append(p[:i], emit)
		if p[i] == '\n' {
			emit(s.cur)
			s.cur = s.cur[:0]
		} else {
			s.cr = true
		}
		p = p[i+1:]
	}
}

func (s *lineSplitter) append(p []byte, emit func(line []byte)) {
	s.cur = append(s.cur, p...)
	for len(s.cur) > maxLineBytes {
		cut := maxLineBytes
		for cut > 0 && !utf8.RuneStart(s.cur[cut]) {
			cut--
		}
		if cut == 0 {
			cut = maxLineBytes
		}
		emit(s.cur[:cut])
		s.cur = append(s.cur[:0], s.cur[cut:]...)
	}
}

// partial returns the current unterminated line.
func (s *lineSplitter) partial() []byte {
	return s.cur
}

// flush emits the unterminated line, if any.
func (s *lineSplitter) flush(emit func(line []byte)) {
	if len(s.cur) > 0 {
		emit(s.cur)
	}
	s.cur = s.cur[:0]
	s.cr = false
}

// splitLines returns output as whole lines, applying the same \r handling.
func splitLines(output []byte) []string {
	var s lineSplitter
	var lines []string
	emit := func(l []byte) { lines = append(lines, string(l)) }
	s.feed(output, emit)
	s.flush(emit)
	return lines
}
