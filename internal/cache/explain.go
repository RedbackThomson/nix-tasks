package cache

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// MaxExplainedFiles bounds each file list printed by Explanation.Write.
const MaxExplainedFiles = 20

// FileChanges lists files that differ between two hashes of a file set.
type FileChanges struct {
	Modified, Added, Removed []string
}

func (c FileChanges) Empty() bool {
	return len(c.Modified)+len(c.Added)+len(c.Removed) == 0
}

func (c FileChanges) summary() string {
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{len(c.Modified), "modified"}, {len(c.Added), "added"}, {len(c.Removed), "removed"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.verb))
		}
	}
	return strings.Join(parts, ", ")
}

func (c FileChanges) lines() []string {
	var lines []string
	for _, p := range c.Modified {
		lines = append(lines, "M "+p)
	}
	for _, p := range c.Added {
		lines = append(lines, "A "+p)
	}
	for _, p := range c.Removed {
		lines = append(lines, "D "+p)
	}
	return lines
}

// DiffFiles compares two sorted file lists.
func DiffFiles(old, cur []FileEntry) FileChanges {
	before := make(map[string]FileEntry, len(old))
	for _, f := range old {
		before[f.Path] = f
	}
	var c FileChanges
	for _, f := range cur {
		prev, ok := before[f.Path]
		switch {
		case !ok:
			c.Added = append(c.Added, f.Path)
		case prev != f:
			c.Modified = append(c.Modified, f.Path)
		}
		delete(before, f.Path)
	}
	c.Removed = slices.Sorted(maps.Keys(before))
	return c
}

// Explanation says why a task missed, relative to an earlier entry.
type Explanation struct {
	// NoPrevious is set when the task has no entry to compare against.
	NoPrevious bool
	// OutputsChanged is set when an entry matched the key but the outputs
	// in the working tree differ from what it recorded.
	OutputsChanged bool
	Outputs        FileChanges

	Definition bool
	Toolchain  bool
	Env        []string
	Inputs     FileChanges
	// Upstream names the dependencies whose keys changed, marked
	// "(derivation)" when a build task's derivation path did.
	Upstream []string
}

// Explain compares the current key with prev, the task's most recently used
// entry (nil when there is none).
func Explain(prev *Entry, cur *Key) Explanation {
	if prev == nil {
		return Explanation{NoPrevious: true}
	}
	x := Explanation{
		Definition: prev.Components.Definition != cur.Components.Definition,
		Toolchain:  prev.Components.Toolchain != cur.Components.Toolchain,
	}
	if prev.Components.Env != cur.Components.Env {
		x.Env = changedKeys(prev.Detail.Env, cur.Detail.Env)
	}
	if prev.Components.Inputs != cur.Components.Inputs {
		x.Inputs = DiffFiles(prev.Detail.Inputs, cur.Detail.Inputs)
	}
	if prev.Components.Upstream != cur.Components.Upstream {
		for _, name := range changedKeys(prev.Detail.Upstream, cur.Detail.Upstream) {
			if prev.Detail.UpstreamDrvs[name] != cur.Detail.UpstreamDrvs[name] {
				name += " (derivation)"
			}
			x.Upstream = append(x.Upstream, name)
		}
	}
	return x
}

// ExplainOutputs explains a miss on an entry whose key matched but whose
// recorded outputs differ from cur.
func ExplainOutputs(entry *Entry, cur []FileEntry) Explanation {
	return Explanation{OutputsChanged: true, Outputs: DiffFiles(entry.Detail.Outputs, cur)}
}

func changedKeys(a, b map[string]string) []string {
	var keys []string
	for k := range a {
		if v, ok := b[k]; !ok || v != a[k] {
			keys = append(keys, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Reason is a one-line summary of the explanation.
func (x Explanation) Reason() string {
	switch {
	case x.NoPrevious:
		return "no previous entry"
	case x.OutputsChanged:
		return "outputs changed since last run"
	}
	var parts []string
	if x.Definition {
		parts = append(parts, "definition changed")
	}
	if x.Toolchain {
		parts = append(parts, "toolchain changed")
	}
	if len(x.Env) > 0 {
		parts = append(parts, "env changed: "+strings.Join(x.Env, ", "))
	}
	if !x.Inputs.Empty() {
		parts = append(parts, "inputs changed: "+x.Inputs.summary())
	}
	if len(x.Upstream) > 0 {
		parts = append(parts, "upstream changed: "+strings.Join(x.Upstream, ", "))
	}
	if len(parts) == 0 {
		return "key changed"
	}
	return strings.Join(parts, "; ")
}

// Write prints the explanation's detail, one component per line in key
// order, with changed files listed beneath.
func (x Explanation) Write(w *strings.Builder, indent string) {
	switch {
	case x.NoPrevious:
		return
	case x.OutputsChanged:
		writeFiles(w, indent+"  ", x.Outputs)
		return
	}
	if x.Definition {
		fmt.Fprintf(w, "%sdefinition changed\n", indent)
	}
	if x.Toolchain {
		fmt.Fprintf(w, "%stoolchain changed\n", indent)
	}
	if len(x.Env) > 0 {
		fmt.Fprintf(w, "%senv changed: %s\n", indent, strings.Join(x.Env, ", "))
	}
	if !x.Inputs.Empty() {
		fmt.Fprintf(w, "%sinputs changed: %s\n", indent, x.Inputs.summary())
		writeFiles(w, indent+"  ", x.Inputs)
	}
	if len(x.Upstream) > 0 {
		fmt.Fprintf(w, "%supstream changed: %s\n", indent, strings.Join(x.Upstream, ", "))
	}
}

func writeFiles(w *strings.Builder, indent string, c FileChanges) {
	lines := c.lines()
	for i, line := range lines {
		if i == MaxExplainedFiles {
			fmt.Fprintf(w, "%s... %d more\n", indent, len(lines)-i)
			break
		}
		fmt.Fprintf(w, "%s%s\n", indent, line)
	}
}
