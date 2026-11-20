package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// OutputMode determines how task output is displayed
type OutputMode int

const (
	// Buffered collects output, shows on error (fallback for non-TTY)
	Buffered OutputMode = iota
	// Streaming shows output in real-time with prefixes (default for CI)
	Streaming
	// Progress shows live spinner + tailing log output (default for interactive TTY)
	Progress
)

// DetectOutputMode returns appropriate mode based on environment
func DetectOutputMode() OutputMode {
	if os.Getenv("CI") != "" {
		return Streaming
	}
	if ui.IsTerminal(os.Stdout) {
		return Progress
	}
	return Buffered
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

	// Strip \r before splitting — tools like curl use \r to overwrite
	// progress in-place, which garbles prefixed output.
	cleaned := bytes.ReplaceAll(p, []byte("\r"), nil)

	lines := bytes.Split(cleaned, []byte("\n"))
	for i, line := range lines {
		if len(line) > 0 || i < len(lines)-1 {
			_, _ = fmt.Fprintf(w.out, "%s%s\n", w.prefix, line)
		}
	}
	return len(p), nil
}
