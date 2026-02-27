package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// Printer handles formatted output
type Printer struct {
	mu      sync.Mutex
	buffers map[string]*bytes.Buffer
}

// NewPrinter creates a new printer
func NewPrinter() *Printer {
	return &Printer{
		buffers: make(map[string]*bytes.Buffer),
	}
}

// TaskBuffer returns a buffer for capturing task output
func (p *Printer) TaskBuffer(name string) io.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()
	buf := &bytes.Buffer{}
	p.buffers[name] = buf
	return buf
}

// TaskStarted prints task start message
func (p *Printer) TaskStarted(name string) {
	// In non-verbose mode, we don't print anything on start
}

// TaskSucceeded prints success message
func (p *Printer) TaskSucceeded(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", Green("✓"), name)
}

// TaskSucceededWithDuration prints success message with duration
func (p *Printer) TaskSucceededWithDuration(name string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s %s %s\n", Green("✓"), name, Gray(FormatDuration(duration)))
}

// TaskFailed prints failure message and buffered output
func (p *Printer) TaskFailed(name string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", Red("✗"), name)

	// Print buffered output if we have it
	if buf, ok := p.buffers[name]; ok && buf.Len() > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", buf.String())
	}
}

// TaskFailedWithDuration prints failure message with duration and buffered output
func (p *Printer) TaskFailedWithDuration(name string, err error, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s %s %s\n", Red("✗"), name, Gray(FormatDuration(duration)))

	// Print buffered output if we have it
	if buf, ok := p.buffers[name]; ok && buf.Len() > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", buf.String())
	}
}

// TaskCached prints cached message
func (p *Printer) TaskCached(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s %s %s\n", Green("✓"), name, Gray("(cached)"))
}

// GetBuffer returns the buffered output for a task (empty if none or if verbose mode streamed it)
func (p *Printer) GetBuffer(name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if buf, ok := p.buffers[name]; ok {
		return buf.String()
	}
	return ""
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("(%dms)", d.Milliseconds())
	}
	return fmt.Sprintf("(%.1fs)", d.Seconds())
}

// PrintTaskList prints a formatted list of tasks
func (p *Printer) PrintTaskList(tasks map[string]config.Task) {
	if len(tasks) == 0 {
		fmt.Println("No tasks defined")
		return
	}

	// Sort task names for consistent output
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("Tasks:")
	for _, name := range names {
		task := tasks[name]
		if task.Description != "" {
			fmt.Printf("  %-20s %s\n", name, task.Description)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}

// PrintShellList prints a formatted list of dev shells
func (p *Printer) PrintShellList(shells map[string]config.Shell) {
	if len(shells) == 0 {
		return
	}

	// Sort shell names for consistent output
	names := make([]string, 0, len(shells))
	for name := range shells {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("\nDev Shells:")
	for _, name := range names {
		shell := shells[name]
		if shell.Extends != "" {
			fmt.Printf("  %-20s extends %s\n", name, shell.Extends)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}
