package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultMaxEntries is how many entries are kept per task unless configured:
// enough for several branches in flight without unbounded growth.
const DefaultMaxEntries = 8

// MaxEntriesEnv overrides every configured retention limit.
const MaxEntriesEnv = "NIX_TASKS_CACHE_MAX_ENTRIES"

// OrphanAge is how long an entry for a task that no longer exists is kept.
const OrphanAge = 30 * 24 * time.Hour

const (
	layoutDir = "v2"
	entryExt  = ".json"
	// A temp file older than this was left by a process that died mid-write.
	staleTempAge = time.Hour
)

// Entry records one successful run of a cached task.
type Entry struct {
	Format   int       `json:"format"`
	Task     string    `json:"task"`
	Key      string    `json:"key"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"lastUsed"`
	// LastHit is zero until the entry first answers a run.
	LastHit      time.Time  `json:"lastHit,omitzero"`
	RunDuration  Duration   `json:"runDuration"`
	Components   Components `json:"components"`
	Detail       Detail     `json:"detail"`
	OutputDigest string     `json:"outputDigest"`
}

// Duration is a time.Duration written as a string such as "41.2s".
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Round(time.Millisecond).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Store is one project's cache directory. Every checkout of a repository
// shares it: nothing in a key depends on where the checkout lives, and a hit
// still requires the outputs in the running checkout to match.
type Store struct {
	dir string
	// Now returns the current time; tests replace it.
	Now func() time.Time
}

// Open returns the store for projectKey under base (see Dir).
func Open(base, projectKey string) *Store {
	return &Store{dir: filepath.Join(base, layoutDir, projectKey), Now: time.Now}
}

// Dir returns the store's directory.
func (s *Store) Dir() string {
	return s.dir
}

// taskDir hashes the task name so names containing "/" or ".." stay inside
// the store.
func (s *Store) taskDir(task string) string {
	sum := sha256.Sum256([]byte(task))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])[:16])
}

func (s *Store) entryPath(task, key string) string {
	return filepath.Join(s.taskDir(task), key+entryExt)
}

// Lookup returns the entry stored for task under key.
func (s *Store) Lookup(task, key string) (*Entry, bool) {
	e, err := s.read(s.entryPath(task, key))
	if err != nil {
		return nil, false
	}
	if e.Task != task || e.Key != key {
		return nil, false
	}
	return e, true
}

// Latest returns the task's most recently used entry.
func (s *Store) Latest(task string) (*Entry, bool) {
	entries := s.Entries(task)
	if len(entries) == 0 {
		return nil, false
	}
	return entries[0], true
}

// Entries returns the task's valid entries, most recently used first.
func (s *Store) Entries(task string) []*Entry {
	entries, _ := s.entriesIn(s.taskDir(task))
	return entries
}

// Touch marks e as having answered a run.
func (s *Store) Touch(e *Entry) error {
	now := s.Now().UTC()
	e.LastUsed = now
	e.LastHit = now
	return s.write(e)
}

// Put stores e and evicts the task's least recently used entries beyond
// maxEntries.
func (s *Store) Put(e *Entry, maxEntries int) error {
	now := s.Now().UTC()
	e.Format = FormatVersion
	if e.Created.IsZero() {
		e.Created = now
	}
	e.LastUsed = now
	if err := s.write(e); err != nil {
		return err
	}
	return s.evict(e.Task, maxEntries)
}

// write replaces the entry file atomically, so a reader sees either the old
// entry or the new one and a crash never leaves a half-written entry.
func (s *Store) write(e *Entry) error {
	dir := s.taskDir(e.Task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, e.Key+entryExt+".tmp-"+strconv.Itoa(os.Getpid())+"-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), s.entryPath(e.Task, e.Key))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing cache entry: %w", err)
	}
	return nil
}

// read parses an entry file. A file that is not a valid entry of this
// format is removed, so it never answers a lookup.
func (s *Store) read(path string) (*Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil || e.Format != FormatVersion || e.Task == "" ||
		e.Key+entryExt != filepath.Base(path) {
		slog.Debug("removing invalid cache entry", "path", path, "error", err)
		_ = os.Remove(path)
		return nil, fmt.Errorf("invalid cache entry %s", path)
	}
	return &e, nil
}

// entriesIn reads every entry in a task directory, most recently used first,
// and removes temp files abandoned by interrupted writes.
func (s *Store) entriesIn(dir string) ([]*Entry, int64) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0
	}
	var entries []*Entry
	var size int64
	for _, f := range files {
		path := filepath.Join(dir, f.Name())
		info, err := f.Info()
		if err != nil {
			continue
		}
		if strings.Contains(f.Name(), entryExt+".tmp-") {
			if s.Now().Sub(info.ModTime()) > staleTempAge {
				_ = os.Remove(path)
			}
			continue
		}
		if !strings.HasSuffix(f.Name(), entryExt) {
			continue
		}
		e, err := s.read(path)
		if err != nil {
			continue
		}
		entries = append(entries, e)
		size += info.Size()
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int { return b.LastUsed.Compare(a.LastUsed) })
	return entries, size
}

func (s *Store) evict(task string, maxEntries int) error {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	entries := s.Entries(task)
	var errs []error
	for _, e := range entries[min(maxEntries, len(entries)):] {
		if err := os.Remove(s.entryPath(e.Task, e.Key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CleanTask removes every entry for task.
func (s *Store) CleanTask(task string) error {
	return os.RemoveAll(s.taskDir(task))
}

// Clean removes every entry in the store.
func (s *Store) Clean() error {
	return os.RemoveAll(s.dir)
}

// RemoveOrphans deletes entries of tasks for which known returns false and
// that have not been used for OrphanAge.
func (s *Store) RemoveOrphans(known func(task string) bool) {
	dirs, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, d := range dirs {
		dir := filepath.Join(s.dir, d.Name())
		entries, _ := s.entriesIn(dir)
		if len(entries) == 0 || known(entries[0].Task) || s.Now().Sub(entries[0].LastUsed) < OrphanAge {
			continue
		}
		slog.Debug("removing cache entries of a removed task", "task", entries[0].Task)
		_ = os.RemoveAll(dir)
	}
}

// TaskStats summarises one task's entries.
type TaskStats struct {
	Task    string
	Entries int
	Size    int64
	// LastHit is zero when no entry has answered a run.
	LastHit     time.Time
	LastRun     time.Time
	RunDuration time.Duration
}

// Stats summarises every task in the store, sorted by task name.
func (s *Store) Stats() []TaskStats {
	dirs, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var stats []TaskStats
	for _, d := range dirs {
		entries, size := s.entriesIn(filepath.Join(s.dir, d.Name()))
		if len(entries) == 0 {
			continue
		}
		st := TaskStats{Task: entries[0].Task, Entries: len(entries), Size: size}
		for _, e := range entries {
			if e.LastHit.After(st.LastHit) {
				st.LastHit = e.LastHit
			}
			if e.Created.After(st.LastRun) {
				st.LastRun = e.Created
				st.RunDuration = time.Duration(e.RunDuration)
			}
		}
		stats = append(stats, st)
	}
	slices.SortFunc(stats, func(a, b TaskStats) int { return strings.Compare(a.Task, b.Task) })
	return stats
}

// MaxEntries resolves the retention limit for a task: the environment
// overrides the task, which overrides the flake-wide setting.
func MaxEntries(taskLimit, flakeLimit int) int {
	if v, err := strconv.Atoi(os.Getenv(MaxEntriesEnv)); err == nil && v > 0 {
		return v
	}
	if taskLimit > 0 {
		return taskLimit
	}
	if flakeLimit > 0 {
		return flakeLimit
	}
	return DefaultMaxEntries
}
