package ui

import (
	"os"
	"strconv"
	"sync/atomic"

	"github.com/charmbracelet/x/term"
)

// Color is one of the 16 standard terminal colours: 0–7 and their bright
// variants 8–15. Using only these lets the terminal's theme pick the shades.
type Color uint8

// Status colours. Decoration never uses them, so they always mean a result.
const (
	SuccessColor Color = 2 // green
	FailureColor Color = 1 // red
	WarningColor Color = 3 // yellow
)

// Decorative colours.
const (
	AccentColor Color = 4 // blue
	LabelColor  Color = 6 // cyan
	MutedColor  Color = 8 // bright black
)

// sgr is the SGR parameter that sets c as the foreground colour.
func (c Color) sgr() string {
	if c < 8 {
		return strconv.Itoa(30 + int(c))
	}
	return strconv.Itoa(90 + int(c) - 8)
}

func (c Color) paint(s string) string { return sgr(c.sgr(), s) }

func Success(s string) string { return SuccessColor.paint(s) }
func Failure(s string) string { return FailureColor.paint(s) }
func Warning(s string) string { return WarningColor.paint(s) }
func Accent(s string) string  { return AccentColor.paint(s) }
func Label(s string) string   { return LabelColor.paint(s) }
func Muted(s string) string   { return MutedColor.paint(s) }

// ColorMode is the value of the --color flag.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

var colorEnabled atomic.Bool

// SetColor turns colour output on or off for the whole process.
func SetColor(on bool) {
	colorEnabled.Store(on)
}

// ColorEnabled reports whether colour output is on.
func ColorEnabled() bool {
	return colorEnabled.Load()
}

// ResolveColor decides whether to colour output written to out.
func ResolveColor(mode ColorMode, out *os.File) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return IsTerminal(out)
}

// IsTerminal reports whether f is a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// sgr wraps s in an SGR sequence with params, such as "1;32" for bold green,
// when colour output is on.
func sgr(params, s string) string {
	if !colorEnabled.Load() {
		return s
	}
	return "\033[" + params + "m" + s + "\033[0m"
}
