package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// labelWidth lines up the values of the detail pane's fields.
const labelWidth = 13

// detail describes one task or dev shell for the detail pane. Each section is
// a list of lines; empty sections are left out.
type detail struct {
	width    int
	sections [][]string
}

func (d *detail) add(lines ...string) {
	if len(lines) > 0 {
		d.sections = append(d.sections, lines)
	}
}

// field returns a label and its value, wrapped under the value's column.
func (d *detail) field(label, value string) []string {
	if value == "" {
		return nil
	}
	indent := strings.Repeat(" ", labelWidth)
	wrapped := strings.Split(ansi.Wrap(value, max(10, d.width-labelWidth), " ,"), "\n")
	lines := []string{labelStyle.Render(fmt.Sprintf("%-*s", labelWidth, label)) + wrapped[0]}
	for _, l := range wrapped[1:] {
		lines = append(lines, indent+l)
	}
	return lines
}

// block returns a label on its own line followed by indented lines.
func (d *detail) block(label string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := []string{labelStyle.Render(label)}
	for _, l := range lines {
		out = append(out, "  "+l)
	}
	return out
}

func (d *detail) lines() []string {
	var out []string
	for i, s := range d.sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, s...)
	}
	return out
}

func taskDetail(name string, task config.Task, width int) []string {
	d := &detail{width: width}

	head := []string{titleStyle.Render(name)}
	if task.Description != "" {
		head = append(head, strings.Split(ansi.Wrap(task.Description, width, ""), "\n")...)
	}
	d.add(head...)

	taskType := task.Type
	if taskType == "" {
		taskType = config.TaskTypeShell
	}
	var fields []string
	fields = append(fields, d.field("Type", string(taskType))...)
	fields = append(fields, d.field("Packages", strings.Join(task.Deps, ", "))...)
	fields = append(fields, d.field("Depends on", strings.Join(task.Depends, ", "))...)
	fields = append(fields, d.field("Runs after", strings.Join(task.After, ", "))...)
	fields = append(fields, d.field("Working dir", task.WorkingDir)...)
	if task.ContinueOnError {
		fields = append(fields, d.field("On failure", "continue (continueOnError)")...)
	}
	d.add(fields...)

	d.add(d.block("Env", envLines(task.Env))...)

	switch {
	case len(task.Commands) > 0:
		d.add(d.block("Commands", task.Commands)...)
	case task.Script != "":
		d.add(d.block("Script", strings.Split(strings.TrimRight(task.Script, "\n"), "\n"))...)
	}
	return d.lines()
}

func shellDetail(name string, shells map[string]config.Shell, width int) []string {
	d := &detail{width: width}
	shell := shells[name]

	d.add(titleStyle.Render(name))
	var fields []string
	fields = append(fields, d.field("Extends", strings.Join(extendsChain(name, shells)[1:], " → "))...)
	fields = append(fields, d.field("Packages", strings.Join(shell.Packages, ", "))...)
	fields = append(fields, d.field("Inherits", strings.Join(inheritedPackages(name, shells), ", "))...)
	d.add(fields...)
	d.add(d.block("Env", envLines(shell.Env))...)
	return d.lines()
}

// extendsChain returns name followed by the shells it extends, nearest first.
func extendsChain(name string, shells map[string]config.Shell) []string {
	chain := []string{name}
	seen := map[string]bool{name: true}
	for parent := shells[name].Extends; parent != "" && !seen[parent]; parent = shells[parent].Extends {
		chain = append(chain, parent)
		seen[parent] = true
	}
	return chain
}

// inheritedPackages lists the packages a shell gets from the shells it
// extends, each once.
func inheritedPackages(name string, shells map[string]config.Shell) []string {
	own := map[string]bool{}
	for _, p := range shells[name].Packages {
		own[p] = true
	}
	var pkgs []string
	for _, parent := range extendsChain(name, shells)[1:] {
		for _, p := range shells[parent].Packages {
			if !own[p] {
				own[p] = true
				pkgs = append(pkgs, p)
			}
		}
	}
	return pkgs
}

func envLines(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + env[k]
	}
	return lines
}
