package ui

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// writeSummary prints the end-of-run summary shared by every reporter: the
// dependency tree of each target with every task's result, the output of each
// failed task (when output returns it, i.e. it was not already shown), task
// warnings, and the totals line.
func writeSummary(w io.Writer, graph *runner.TaskGraph, targets []string, results []runner.TaskResult, output func(name string) ([]byte, bool)) {
	var b strings.Builder
	b.WriteString("\n")
	for _, line := range resultTree(graph, targets, results) {
		b.WriteString(line + "\n")
	}

	for _, r := range results {
		if !failed(r) {
			continue
		}
		out, ok := output(r.Name)
		if !ok || (len(bytes.TrimSpace(out)) == 0 && errorDetail(r) == "") {
			continue
		}
		fmt.Fprintf(&b, "\n%s %s\n", StateIcon(r.State), r.Name+" output:")
		writeFailure(&b, r, out)
	}

	first := true
	for _, r := range results {
		for _, warning := range r.Warnings {
			if first {
				b.WriteString("\n")
				first = false
			}
			b.WriteString(formatWarning(r.Name, warning) + "\n")
		}
	}

	fmt.Fprintf(&b, "\n%s\n", completedLine(results))
	_, _ = io.WriteString(w, b.String())
}

// completedLine totals the run, counting cache hits apart from tasks that
// ran and passed, with the time the hits saved.
func completedLine(results []runner.TaskResult) string {
	var passed, cached int
	var saved time.Duration
	counts := map[runner.TaskState]int{}
	for _, r := range results {
		switch {
		case r.Cached:
			cached++
			saved += r.Saved
		case r.State == runner.Succeeded:
			passed++
		default:
			counts[r.State]++
		}
	}
	var parts []string
	if passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", passed))
	}
	switch {
	case cached > 0 && saved > 0:
		parts = append(parts, fmt.Sprintf("%d cached (%s saved)", cached, ShortDuration(saved)))
	case cached > 0:
		parts = append(parts, fmt.Sprintf("%d cached", cached))
	}
	if n := counts[runner.Failed]; n > 0 {
		parts = append(parts, Failure(fmt.Sprintf("%d failed", n)))
	}
	if n := counts[runner.FailedAllowed]; n > 0 {
		parts = append(parts, Warning(fmt.Sprintf("%d failed (allowed)", n)))
	}
	if n := counts[runner.Skipped]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", n))
	}
	if n := counts[runner.Interrupted]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d interrupted", n))
	}
	line := fmt.Sprintf("Completed: %d tasks", len(results))
	if len(parts) > 0 {
		line += fmt.Sprintf(" (%s)", strings.Join(parts, ", "))
	}
	return line
}

// resultTree draws one tree per target, each task under the tasks that
// depend on it, with its after-hooks beneath it. A task reached a second
// time, from another target or a shared dependency, is marked with * and not
// expanded again.
func resultTree(graph *runner.TaskGraph, targets []string, results []runner.TaskResult) []string {
	t := summaryTree{graph: graph, results: map[string]runner.TaskResult{}, shown: map[string]bool{}}
	for _, r := range results {
		t.results[r.Name] = r
	}
	for _, target := range targets {
		t.node(target, "", "", false, "")
	}
	return t.lines
}

type summaryTree struct {
	graph   *runner.TaskGraph
	results map[string]runner.TaskResult
	shown   map[string]bool
	lines   []string
}

type summaryChild struct {
	name    string
	isAfter bool
}

// node draws name after linePrefix, then its children indented by
// childPrefix. parent is the node above it in the tree.
func (t *summaryTree) node(name, linePrefix, childPrefix string, isAfter bool, parent string) {
	repeat := t.shown[name]
	t.shown[name] = true

	label := name
	if isAfter {
		label += " " + Label("(after)")
	}
	if r, ok := t.results[name]; ok {
		label += " " + StateIcon(r.State) + resultDetail(r)
	}
	if repeat {
		label += " " + Muted("*")
	}
	t.lines = append(t.lines, linePrefix+label)
	if repeat {
		return
	}

	children := t.children(name, parent)
	for i, c := range children {
		branch, indent := "├─ ", "│  "
		if i == len(children)-1 {
			branch, indent = "└─ ", "   "
		}
		t.node(c.name, childPrefix+Muted(branch), childPrefix+Muted(indent), c.isAfter, name)
	}
}

// children returns name's dependencies and the after-hooks of name that ran.
// A hook depends on the task it follows; that edge back to the tree parent is
// left out, since the tree already shows it.
func (t *summaryTree) children(name, parent string) []summaryChild {
	deps := append([]string(nil), t.graph.Dependencies(name)...)
	sort.Strings(deps)
	hooks := append([]string(nil), t.graph.AfterHooks(name)...)
	sort.Strings(hooks)

	var children []summaryChild
	for _, dep := range deps {
		if dep != parent {
			children = append(children, summaryChild{name: dep})
		}
	}
	for _, hook := range hooks {
		if _, ran := t.results[hook]; ran {
			children = append(children, summaryChild{name: hook, isAfter: true})
		}
	}
	return children
}
