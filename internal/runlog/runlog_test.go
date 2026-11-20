package runlog_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/runlog"
)

func TestRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := runlog.Root(); got != "/state/nix-tasks/runs" {
		t.Errorf("Root() = %q with XDG_STATE_HOME set", got)
	}

	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/me")
	if got, _ := runlog.Root(); got != "/home/me/.local/state/nix-tasks/runs" {
		t.Errorf("Root() = %q without XDG_STATE_HOME", got)
	}
}

func TestNewID_SortsByTime(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	var ids []string
	for _, d := range []time.Duration{0, time.Millisecond, time.Second, time.Hour} {
		id, err := runlog.NewID(base.Add(d))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if !strings.HasPrefix(ids[0], "20260102T020405.000Z-") {
		t.Errorf("ID should be the UTC start time, got %s", ids[0])
	}
	if !slices.IsSorted(ids) {
		t.Errorf("IDs should sort by start time: %v", ids)
	}
}

func TestStart_PrunesOldRuns(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var dirs []string
	for i := range 5 {
		dir, err := runlog.Start(root, base.Add(time.Duration(i)*time.Minute), 3)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	// Not a run directory, so never removed.
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runlog.Prune(root, 3); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, filepath.Join(root, e.Name()))
	}
	want := append(slices.Clone(dirs[2:]), filepath.Join(root, "notes"))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

func TestPrune_MissingRoot(t *testing.T) {
	if err := runlog.Prune(filepath.Join(t.TempDir(), "absent"), 3); err == nil {
		t.Error("expected an error for a missing root")
	}
}
