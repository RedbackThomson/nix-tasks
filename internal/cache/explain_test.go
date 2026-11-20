package cache

import (
	"fmt"
	"strings"
	"testing"
)

func TestExplain(t *testing.T) {
	f := newFixture(t)
	prev := f.key()
	entry := &Entry{Components: prev.Components, Detail: prev.Detail}

	f.write("apis/v1/types.go", "edited")
	f.write("apis/v1/new.go", "new")
	f.env["GOFLAGS"] = "-x"
	f.req.Upstream = map[string]string{"tools": "k2", "more": "k3"}
	f.req.Task.ShellDrvPath = "/nix/store/c-nix-shell.drv"
	f.req.Task.Commands = []string{"other"}

	x := Explain(entry, f.key())
	var buf strings.Builder
	x.Write(&buf, "  ")
	want := `  definition changed
  toolchain changed
  env changed: GOFLAGS
  inputs changed: 1 modified, 1 added
    M apis/v1/types.go
    A apis/v1/new.go
  upstream changed: more, tools
`
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
	if r := x.Reason(); r != "definition changed; toolchain changed; env changed: GOFLAGS; inputs changed: 1 modified, 1 added; upstream changed: more, tools" {
		t.Errorf("reason %q", r)
	}
	if r := Explain(nil, f.key()).Reason(); r != "no previous entry" {
		t.Errorf("reason %q", r)
	}
}

func TestExplain_BuildUpstreamDerivation(t *testing.T) {
	f := newFixture(t)
	f.req.Upstream = map[string]string{"tool": BuildKey("/nix/store/a.drv", nil), "gen": "k1"}
	f.req.UpstreamDrvs = map[string]string{"tool": "/nix/store/a.drv"}
	prev := f.key()
	entry := &Entry{Components: prev.Components, Detail: prev.Detail}

	f.req.Upstream = map[string]string{"tool": BuildKey("/nix/store/b.drv", nil), "gen": "k2"}
	f.req.UpstreamDrvs = map[string]string{"tool": "/nix/store/b.drv"}
	if r := Explain(entry, f.key()).Reason(); r != "upstream changed: gen, tool (derivation)" {
		t.Errorf("reason %q", r)
	}
	if BuildKey("/nix/store/a.drv", nil) == BuildKey("/nix/store/a.drv", map[string]string{"x": "k"}) {
		t.Error("a build task's own upstream keys should reach its key")
	}
}

func TestExplainOutputs(t *testing.T) {
	entry := &Entry{Detail: Detail{Outputs: []FileEntry{{"a", KindFile, "1"}, {"b", KindFile, "2"}}}}
	x := ExplainOutputs(entry, []FileEntry{{"a", KindFile, "9"}})
	var buf strings.Builder
	x.Write(&buf, "  ")
	if x.Reason() != "outputs changed since last run" || buf.String() != "    M a\n    D b\n" {
		t.Errorf("reason %q, detail %q", x.Reason(), buf.String())
	}
}

func TestExplain_TruncatesFileLists(t *testing.T) {
	var files []FileEntry
	for i := range MaxExplainedFiles + 5 {
		files = append(files, FileEntry{Path: fmt.Sprintf("f%02d", i), Kind: KindFile, Hash: "h"})
	}
	x := Explanation{Inputs: DiffFiles(nil, files)}
	var buf strings.Builder
	x.Write(&buf, "")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != MaxExplainedFiles+2 || lines[len(lines)-1] != "  ... 5 more" {
		t.Errorf("got %d lines, last %q", len(lines), lines[len(lines)-1])
	}
}
