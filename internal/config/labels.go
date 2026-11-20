package config

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxLabelLength is the longest task name shown in full in streamed output;
// longer names without a label are abbreviated.
const MaxLabelLength = 24

// Labels returns the display label of every task: its label if set, its
// name if that is at most MaxLabelLength characters, and otherwise its name
// with the leading words cut to initials. Labels depend only on the config,
// never on which tasks run, so a task is always shown the same way.
func Labels(tasks map[string]Task) map[string]string {
	labels := make(map[string]string, len(tasks))
	keep := map[string]int{}
	for name, task := range tasks {
		switch {
		case task.Label != "":
			labels[name] = task.Label
		case utf8.RuneCountInString(name) <= MaxLabelLength:
			labels[name] = name
		default:
			keep[name] = 2
			labels[name] = abbreviate(name, 2)
		}
	}

	// Two abbreviations, or an abbreviation and another label, can match;
	// every abbreviated task in the clash keeps one more word whole until
	// they differ. A name fully spelled out cannot grow, which only happens
	// when a label equals another task's name, a validation error.
	for {
		byLabel := map[string][]string{}
		for name, label := range labels {
			byLabel[label] = append(byLabel[label], name)
		}
		grew := false
		for _, names := range byLabel {
			if len(names) < 2 {
				continue
			}
			for _, name := range names {
				k, ok := keep[name]
				if !ok || k >= len(splitWords(name).words) {
					continue
				}
				keep[name] = k + 1
				labels[name] = abbreviate(name, k+1)
				grew = true
			}
		}
		if !grew {
			return labels
		}
	}
}

type words struct {
	words []string
	seps  []string // seps[i] follows words[i]
}

func isLabelSeparator(r rune) bool {
	return strings.ContainsRune("-_.:/", r)
}

func splitWords(name string) words {
	var w words
	start := 0
	for i, r := range name {
		if isLabelSeparator(r) {
			w.words = append(w.words, name[start:i])
			w.seps = append(w.seps, string(r))
			start = i + utf8.RuneLen(r)
		}
	}
	w.words = append(w.words, name[start:])
	return w
}

// abbreviate cuts all but the last keep words of name to their first letter.
func abbreviate(name string, keep int) string {
	w := splitWords(name)
	var b strings.Builder
	for i, word := range w.words {
		if i < len(w.words)-keep {
			if r, size := utf8.DecodeRuneInString(word); size > 0 {
				b.WriteRune(r)
			}
		} else {
			b.WriteString(word)
		}
		if i < len(w.seps) {
			b.WriteString(w.seps[i])
		}
	}
	return b.String()
}

// labelErrors reports labels that would make two tasks look the same.
func labelErrors(tasks map[string]Task) []string {
	var errs []string
	owner := map[string]string{}
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		label := tasks[name].Label
		if label == "" {
			continue
		}
		if other, ok := owner[label]; ok {
			errs = append(errs, fmt.Sprintf("task '%s': label '%s' is already used by task '%s'", name, label, other))
			continue
		}
		owner[label] = name
		if _, ok := tasks[label]; ok && label != name {
			errs = append(errs, fmt.Sprintf("task '%s': label '%s' is the name of another task", name, label))
		}
	}
	return errs
}

// LabelWarnings names every task whose name is too long to be shown in full
// and has no label, with the abbreviation it gets instead.
func LabelWarnings(tasks map[string]Task) []string {
	labels := Labels(tasks)
	var warnings []string
	for name, task := range tasks {
		if task.Label == "" && labels[name] != name {
			warnings = append(warnings, fmt.Sprintf(
				"task '%s': name is longer than %d characters, so streamed output abbreviates it to '%s'; set `label` to choose a shorter name",
				name, MaxLabelLength, labels[name]))
		}
	}
	slices.Sort(warnings)
	return warnings
}
