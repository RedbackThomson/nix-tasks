package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func TestDir(t *testing.T) {
	// Save original env vars
	origCache := os.Getenv("NIX_TASKS_CACHE_DIR")
	origXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		if origCache != "" {
			os.Setenv("NIX_TASKS_CACHE_DIR", origCache)
		} else {
			os.Unsetenv("NIX_TASKS_CACHE_DIR")
		}
		if origXDG != "" {
			os.Setenv("XDG_CACHE_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CACHE_HOME")
		}
	}()

	t.Run("uses NIX_TASKS_CACHE_DIR if set", func(t *testing.T) {
		os.Setenv("NIX_TASKS_CACHE_DIR", "/custom/cache")
		os.Unsetenv("XDG_CACHE_HOME")

		got := Dir()
		if got != "/custom/cache" {
			t.Errorf("Dir() = %q, want %q", got, "/custom/cache")
		}
	})

	t.Run("uses XDG_CACHE_HOME if NIX_TASKS_CACHE_DIR not set", func(t *testing.T) {
		os.Unsetenv("NIX_TASKS_CACHE_DIR")
		os.Setenv("XDG_CACHE_HOME", "/xdg/cache")

		got := Dir()
		want := "/xdg/cache/nix-tasks"
		if got != want {
			t.Errorf("Dir() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.cache/nix-tasks", func(t *testing.T) {
		os.Unsetenv("NIX_TASKS_CACHE_DIR")
		os.Unsetenv("XDG_CACHE_HOME")

		got := Dir()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".cache", "nix-tasks")
		if got != want {
			t.Errorf("Dir() = %q, want %q", got, want)
		}
	})
}

func TestComputeFingerprint(t *testing.T) {
	// Most cases need a non-empty Inputs list because shell tasks without
	// declared inputs are intentionally not cached.
	withInputs := func(t config.Task) config.Task {
		t.Inputs = []string{"**/*.go"}
		return t
	}

	t.Run("same task produces same fingerprint", func(t *testing.T) {
		task := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
		})
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp1, err := ComputeFingerprint(task, packages, "")
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}

		fp2, err := ComputeFingerprint(task, packages, "")
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}

		if fp1.Hash != fp2.Hash {
			t.Errorf("Fingerprints differ: %s != %s", fp1.Hash, fp2.Hash)
		}
	})

	t.Run("different commands produce different fingerprint", func(t *testing.T) {
		task1 := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
		})
		task2 := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go test ./..."},
		})
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp1, _ := ComputeFingerprint(task1, packages, "")
		fp2, _ := ComputeFingerprint(task2, packages, "")

		if fp1.Hash == fp2.Hash {
			t.Error("Fingerprints should differ for different commands")
		}
	})

	t.Run("different packages produce different fingerprint", func(t *testing.T) {
		task := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
		})
		packages1 := map[string]string{
			"go": "/nix/store/abc123-go-1.21",
		}
		packages2 := map[string]string{
			"go": "/nix/store/def456-go-1.22",
		}

		fp1, _ := ComputeFingerprint(task, packages1, "")
		fp2, _ := ComputeFingerprint(task, packages2, "")

		if fp1.Hash == fp2.Hash {
			t.Error("Fingerprints should differ for different packages")
		}
	})

	t.Run("description does not affect fingerprint", func(t *testing.T) {
		task1 := withInputs(config.Task{
			Description: "Build the project",
			Deps:        []string{"go"},
			Commands:    []string{"go build ./..."},
		})
		task2 := withInputs(config.Task{
			Description: "Different description",
			Deps:        []string{"go"},
			Commands:    []string{"go build ./..."},
		})
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp1, _ := ComputeFingerprint(task1, packages, "")
		fp2, _ := ComputeFingerprint(task2, packages, "")

		if fp1.Hash != fp2.Hash {
			t.Error("Description should not affect fingerprint")
		}
	})

	t.Run("env vars affect fingerprint", func(t *testing.T) {
		task1 := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
			Env:      map[string]string{"CGO_ENABLED": "0"},
		})
		task2 := withInputs(config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
			Env:      map[string]string{"CGO_ENABLED": "1"},
		})
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp1, _ := ComputeFingerprint(task1, packages, "")
		fp2, _ := ComputeFingerprint(task2, packages, "")

		if fp1.Hash == fp2.Hash {
			t.Error("Fingerprints should differ for different env vars")
		}
	})

	t.Run("shell task without inputs is not cached", func(t *testing.T) {
		task := config.Task{
			Type:     config.TaskTypeShell,
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
		}
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp, err := ComputeFingerprint(task, packages, "")
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}
		if fp != nil {
			t.Errorf("expected nil fingerprint for shell task without inputs, got %s", fp.Hash)
		}
	})

	t.Run("untyped task without inputs is not cached", func(t *testing.T) {
		// Tasks without an explicit Type are treated as shell-shaped.
		task := config.Task{
			Deps:     []string{"go"},
			Commands: []string{"go build ./..."},
		}
		packages := map[string]string{
			"go": "/nix/store/abc123-go",
		}

		fp, _ := ComputeFingerprint(task, packages, "")
		if fp != nil {
			t.Errorf("expected nil fingerprint for untyped task without inputs, got %s", fp.Hash)
		}
	})

	t.Run("build task without inputs is still cached", func(t *testing.T) {
		task := config.Task{
			Type:    config.TaskTypeBuild,
			DrvPath: "/nix/store/abc123.drv",
		}

		fp, err := ComputeFingerprint(task, nil, "")
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}
		if fp == nil {
			t.Error("expected non-nil fingerprint for build task")
		}
	})

	t.Run("noCache wins over declared inputs", func(t *testing.T) {
		task := config.Task{
			Type:     config.TaskTypeShell,
			Commands: []string{"go test ./..."},
			Inputs:   []string{"**/*.go"},
			NoCache:  true,
		}

		fp, _ := ComputeFingerprint(task, nil, "")
		if fp != nil {
			t.Errorf("expected nil fingerprint when NoCache=true, got %s", fp.Hash)
		}
	})

	t.Run("input file content affects fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "main.go")

		task := config.Task{
			Type:     config.TaskTypeShell,
			Commands: []string{"go build"},
			Inputs:   []string{"main.go"},
		}

		if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		fp1, err := ComputeFingerprint(task, nil, dir)
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}

		if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		fp2, err := ComputeFingerprint(task, nil, dir)
		if err != nil {
			t.Fatalf("ComputeFingerprint() error = %v", err)
		}

		if fp1.Hash == fp2.Hash {
			t.Error("expected fingerprint to change when input file content changes")
		}
	})
}

func TestStore(t *testing.T) {
	// Use a temp directory for testing
	tmpDir := t.TempDir()
	os.Setenv("NIX_TASKS_CACHE_DIR", tmpDir)
	defer os.Unsetenv("NIX_TASKS_CACHE_DIR")

	store := NewStore("test-project")

	t.Run("lookup returns false for missing entry", func(t *testing.T) {
		fp := &Fingerprint{Hash: "nonexistent"}
		_, found := store.Lookup("task", fp)
		if found {
			t.Error("Expected not found for nonexistent entry")
		}
	})

	t.Run("store and lookup success", func(t *testing.T) {
		fp := &Fingerprint{Hash: "abc123"}

		err := store.Store("build", fp, true)
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}

		entry, found := store.Lookup("build", fp)
		if !found {
			t.Fatal("Expected to find stored entry")
		}
		if entry.Fingerprint != fp.Hash {
			t.Errorf("Fingerprint = %q, want %q", entry.Fingerprint, fp.Hash)
		}
		if !entry.Success {
			t.Error("Success = false, want true")
		}
	})

	t.Run("store and lookup failure", func(t *testing.T) {
		fp := &Fingerprint{Hash: "def456"}

		err := store.Store("test", fp, false)
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}

		entry, found := store.Lookup("test", fp)
		if !found {
			t.Fatal("Expected to find stored entry")
		}
		if entry.Success {
			t.Error("Success = true, want false")
		}
	})

	t.Run("clear removes entries", func(t *testing.T) {
		fp := &Fingerprint{Hash: "clear-test"}
		store.Store("cleartest", fp, true)

		err := store.Clear()
		if err != nil {
			t.Fatalf("Clear() error = %v", err)
		}

		_, found := store.Lookup("cleartest", fp)
		if found {
			t.Error("Expected entry to be cleared")
		}
	})
}

func TestProjectKey(t *testing.T) {
	t.Run("same path produces same key", func(t *testing.T) {
		key1 := ProjectKey("/home/user/project")
		key2 := ProjectKey("/home/user/project")

		if key1 != key2 {
			t.Errorf("Keys differ: %s != %s", key1, key2)
		}
	})

	t.Run("different paths produce different keys", func(t *testing.T) {
		key1 := ProjectKey("/home/user/project1")
		key2 := ProjectKey("/home/user/project2")

		if key1 == key2 {
			t.Error("Keys should differ for different paths")
		}
	})

	t.Run("key is a valid directory name", func(t *testing.T) {
		key := ProjectKey("/some/path/with spaces/and:special!chars")

		// Key should be hex-encoded and not contain special chars
		for _, c := range key {
			if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
				t.Errorf("Key contains invalid character: %c", c)
			}
		}
	})
}

func TestStats(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("NIX_TASKS_CACHE_DIR", tmpDir)
	defer os.Unsetenv("NIX_TASKS_CACHE_DIR")

	store := NewStore("stats-test")

	t.Run("empty cache returns zero stats", func(t *testing.T) {
		stats, err := store.Stats()
		if err != nil {
			t.Fatalf("Stats() error = %v", err)
		}
		if stats.EntryCount != 0 {
			t.Errorf("EntryCount = %d, want 0", stats.EntryCount)
		}
	})

	t.Run("stats reflect stored entries", func(t *testing.T) {
		store.Store("task1", &Fingerprint{Hash: "hash1"}, true)
		store.Store("task2", &Fingerprint{Hash: "hash2"}, true)

		stats, err := store.Stats()
		if err != nil {
			t.Fatalf("Stats() error = %v", err)
		}
		if stats.EntryCount != 2 {
			t.Errorf("EntryCount = %d, want 2", stats.EntryCount)
		}
		if stats.TotalSize == 0 {
			t.Error("TotalSize should be > 0")
		}
	})
}
