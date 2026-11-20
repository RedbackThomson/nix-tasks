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

// RunPlan describes a run before any task starts.
type RunPlan struct {
	// Targets are the tasks the run was asked for.
	Targets []string
	// Tasks is the execution set with every task after its dependencies and
	// ties broken by name, so the same pipeline always lists the same way.
	Tasks []string
	// Labels maps every task in the config to its display label.
	Labels map[string]string
	Jobs   int
}

// PlanReporter is implemented by reporters that lay out output for the
// whole run, such as aligning prefixes to the longest label. RunPlanned is
// called once, before anything else.
type PlanReporter interface {
	RunPlanned(plan RunPlan)
}
