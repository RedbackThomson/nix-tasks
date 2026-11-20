package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s := Open(t.TempDir(), "project")
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	return s, &now
}

func entry(task, key string) *Entry {
	return &Entry{Task: task, Key: key, RunDuration: Duration(2 * time.Second), OutputDigest: "d"}
}

func TestStore_PutLookupTouch(t *testing.T) {
	s, now := newTestStore(t)
	if err := s.Put(entry("gen", "k1"), 8); err != nil {
		t.Fatal(err)
	}
	e, ok := s.Lookup("gen", "k1")
	if !ok || e.Format != FormatVersion || time.Duration(e.RunDuration) != 2*time.Second || !e.LastHit.IsZero() {
		t.Fatalf("lookup: %+v %v", e, ok)
	}
	if _, ok := s.Lookup("gen", "k2"); ok {
		t.Error("unknown key found")
	}
	if _, ok := s.Lookup("other", "k1"); ok {
		t.Error("key found under another task")
	}
	*now = now.Add(time.Hour)
	if err := s.Touch(e); err != nil {
		t.Fatal(err)
	}
	e, _ = s.Lookup("gen", "k1")
	if !e.LastUsed.Equal(*now) || !e.LastHit.Equal(*now) {
		t.Errorf("touch did not update times: %+v", e)
	}
}

func TestStore_InterruptedWriteIsNeverRead(t *testing.T) {
	s, _ := newTestStore(t)
	dir := s.taskDir("gen")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "k1.json.tmp-1-2"), []byte(`{"format":2,"task":"gen","key":"k1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup("gen", "k1"); ok {
		t.Error("temp file read as an entry")
	}
	if len(s.Entries("gen")) != 0 {
		t.Error("temp file listed as an entry")
	}
}

func TestStore_CorruptEntryIsMissAndRemoved(t *testing.T) {
	s, _ := newTestStore(t)
	for name, content := range map[string]string{
		"garbage":      "{not json",
		"wrong format": `{"format":1,"task":"gen","key":"k1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := s.entryPath("gen", "k1")
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, ok := s.Lookup("gen", "k1"); ok {
				t.Error("corrupt entry answered a lookup")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Error("corrupt entry not removed")
			}
		})
	}
}

func TestStore_ConcurrentPutsLeaveOneValidEntry(t *testing.T) {
	s, _ := newTestStore(t)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Put(entry("gen", "k1"), 8); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	files, _ := os.ReadDir(s.taskDir("gen"))
	if len(files) != 1 || files[0].Name() != "k1.json" {
		t.Errorf("files %v", files)
	}
	if _, ok := s.Lookup("gen", "k1"); !ok {
		t.Error("entry unreadable")
	}
}

func TestStore_EvictsLeastRecentlyUsed(t *testing.T) {
	s, now := newTestStore(t)
	for i := range 10 {
		*now = now.Add(time.Minute)
		if err := s.Put(entry("gen", fmt.Sprintf("k%d", i)), DefaultMaxEntries); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			continue
		}
		// Keep k0 in use so it survives although it is the oldest.
		e, _ := s.Lookup("gen", "k0")
		_ = s.Touch(e)
	}
	entries := s.Entries("gen")
	if len(entries) != DefaultMaxEntries {
		t.Fatalf("kept %d entries", len(entries))
	}
	var keys []string
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, " ") != "k0 k9 k8 k7 k6 k5 k4 k3" {
		t.Errorf("kept %v", keys)
	}
}

func TestStore_TaskNamesStayInsideStore(t *testing.T) {
	s, _ := newTestStore(t)
	for _, name := range []string{"../../escape", "ns/task", "/abs"} {
		if err := s.Put(entry(name, "k"), 8); err != nil {
			t.Fatal(err)
		}
		dir := s.taskDir(name)
		if filepath.Dir(dir) != s.Dir() {
			t.Errorf("%q stored in %s", name, dir)
		}
		if _, ok := s.Lookup(name, "k"); !ok {
			t.Errorf("%q not found", name)
		}
	}
}

func TestStore_CleanTaskAndOrphans(t *testing.T) {
	s, now := newTestStore(t)
	for _, task := range []string{"gen", "lint", "removed"} {
		_ = s.Put(entry(task, "k"), 8)
	}
	if err := s.CleanTask("lint"); err != nil {
		t.Fatal(err)
	}
	if len(s.Entries("lint")) != 0 || len(s.Entries("gen")) != 1 {
		t.Error("clean --task removed the wrong entries")
	}

	known := func(task string) bool { return task == "gen" }
	s.RemoveOrphans(known)
	if len(s.Entries("removed")) != 1 {
		t.Error("recent orphan removed")
	}
	*now = now.Add(OrphanAge + time.Hour)
	s.RemoveOrphans(known)
	if len(s.Entries("removed")) != 0 || len(s.Entries("gen")) != 1 {
		t.Error("old orphan kept or known task removed")
	}
}

func TestStore_Stats(t *testing.T) {
	s, now := newTestStore(t)
	_ = s.Put(entry("gen", "k1"), 8)
	*now = now.Add(time.Minute)
	e := entry("gen", "k2")
	e.RunDuration = Duration(41 * time.Second)
	_ = s.Put(e, 8)
	_ = s.Touch(e)
	_ = s.Put(entry("lint", "k1"), 8)

	stats := s.Stats()
	if len(stats) != 2 || stats[0].Task != "gen" || stats[0].Entries != 2 || stats[0].Size == 0 ||
		stats[0].RunDuration != 41*time.Second || !stats[0].LastHit.Equal(*now) || !stats[1].LastHit.IsZero() {
		t.Errorf("stats %+v", stats)
	}
}

func TestMaxEntries(t *testing.T) {
	if got := MaxEntries(0, 0); got != DefaultMaxEntries {
		t.Errorf("default %d", got)
	}
	if got := MaxEntries(0, 16); got != 16 {
		t.Errorf("flake setting %d", got)
	}
	if got := MaxEntries(32, 16); got != 32 {
		t.Errorf("task setting %d", got)
	}
	t.Setenv(MaxEntriesEnv, "4")
	if got := MaxEntries(32, 16); got != 4 {
		t.Errorf("environment %d", got)
	}
}

func TestNormalizeRemote(t *testing.T) {
	want := "github.com/org/repo"
	for _, remote := range []string{
		"https://github.com/org/repo.git",
		"https://user:token@GitHub.com/org/repo/",
		"git@github.com:org/repo.git",
		"ssh://git@github.com/org/repo",
	} {
		if got := normalizeRemote(remote); got != want {
			t.Errorf("%s -> %s", remote, got)
		}
	}
}
