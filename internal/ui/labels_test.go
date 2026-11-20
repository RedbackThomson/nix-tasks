package ui

import (
	"bytes"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

const longName = "generate-crossplane-provider-aws-ec2-managed-resources-crds"

var labelPlan = runner.RunPlan{
	Tasks:  []string{longName, "lint"},
	Labels: map[string]string{longName: "g-c-p-a-e-m-resources-crds", "lint": "lint"},
}

func TestLineReporter_StreamUsesLabels(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, true)
	r.RunPlanned(labelPlan)
	r.TaskStarted(longName)
	r.TaskStarted("lint")
	write(t, r, longName, "wrote crds\n", "")
	write(t, r, "lint", "ok\n", "")
	r.TaskFinished(runner.TaskResult{Name: longName, Duration: time.Second})

	want := "▶ g-c-p-a-e-m-resources-crds = " + longName + "\n" +
		"▶ lint\n" +
		"[g-c-p-a-e-m-resources-crds] wrote crds\n" +
		"[lint] ok\n" +
		"✓ " + longName + " (1.0s)\n"
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLineReporter_PlainKeepsNames(t *testing.T) {
	var out bytes.Buffer
	r := NewLineReporter(&out, false)
	r.RunPlanned(labelPlan)
	r.TaskStarted(longName)
	if want := "▶ " + longName + "\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestProgressReporter_UsesLabels(t *testing.T) {
	var out bytes.Buffer
	now := t0
	p := newTestProgress(&out, 80, 24, &now)
	p.RunPlanned(labelPlan)
	p.TaskStarted(longName)
	p.render()
	if want := "⠋ g-c-p-a-e-m-resources-crds (0ms)\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}
