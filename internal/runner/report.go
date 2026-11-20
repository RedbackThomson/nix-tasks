package runner

import (
	"io"
)

// Reporter receives every task's lifecycle events and output. TaskStarted
// and TaskOutput are called from the goroutines running tasks, so
// implementations must be safe for concurrent use. TaskFinished is called once
// per task in the execution set, in completion order, after the task's
// process has exited and all of its output has been written.
type Reporter interface {
	// TaskStarted is called when a task begins executing; cache hits and
	// skipped tasks are never started.
	TaskStarted(name string)
	// TaskOutput returns the writers a task's process writes to.
	TaskOutput(name string) (stdout, stderr io.Writer)
	TaskFinished(r TaskResult)
}

type discardReporter struct{}

func (discardReporter) TaskStarted(string) {}

func (discardReporter) TaskOutput(string) (io.Writer, io.Writer) {
	return io.Discard, io.Discard
}

func (discardReporter) TaskFinished(TaskResult) {}
