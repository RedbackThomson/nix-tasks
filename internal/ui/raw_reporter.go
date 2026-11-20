package ui

import (
	"io"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// RawReporter connects the target task straight to stdout and stderr and
// prints nothing else, except that when a dependency fails the summary goes to
// stderr, since otherwise nothing would say why the target never ran.
type RawReporter struct {
	target         string
	stdout, stderr io.Writer

	mu      sync.Mutex
	outputs outputBuffers
}

// NewRawReporter creates a RawReporter for target.
func NewRawReporter(target string, stdout, stderr io.Writer) *RawReporter {
	return &RawReporter{target: target, stdout: stdout, stderr: stderr}
}

func (r *RawReporter) Start()             {}
func (r *RawReporter) Interrupt()         {}
func (r *RawReporter) Stop()              {}
func (r *RawReporter) TaskStarted(string) {}

func (r *RawReporter) TaskOutput(name string) (io.Writer, io.Writer) {
	if name == r.target {
		return r.stdout, r.stderr
	}
	w := r.outputs.writer(name)
	return w, w
}

func (r *RawReporter) TaskFinished(res runner.TaskResult) {
	if res.Name == r.target || !failed(res) {
		r.outputs.drop(res.Name)
	}
}

func (r *RawReporter) Summary(graph *runner.TaskGraph, targets []string, results []runner.TaskResult) {
	depFailed := false
	for _, res := range results {
		if res.Name != r.target && res.State == runner.Failed {
			depFailed = true
		}
	}
	if !depFailed {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	writeSummary(r.stderr, graph, targets, results, func(name string) ([]byte, bool) {
		if name == r.target {
			return nil, false
		}
		return r.outputs.get(name)
	})
}
