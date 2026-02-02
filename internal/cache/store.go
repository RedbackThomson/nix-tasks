package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a cached task result
type Entry struct {
	Fingerprint string    `json:"fingerprint"`
	Success     bool      `json:"success"`
	Timestamp   time.Time `json:"timestamp"`
}

// Store manages the task result cache
type Store struct {
	dir        string
	projectKey string
}

// NewStore creates a cache store for a project
func NewStore(projectKey string) *Store {
	return &Store{
		dir:        Dir(),
		projectKey: projectKey,
	}
}

// ProjectKey generates a cache key from a flake path
func ProjectKey(flakePath string) string {
	// Use absolute path for consistency
	absPath, err := filepath.Abs(flakePath)
	if err != nil {
		absPath = flakePath
	}

	// Hash the path for a shorter, safe directory name
	h := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(h[:8]) // First 8 bytes = 16 hex chars
}

func (s *Store) taskDir(taskName string) string {
	return filepath.Join(s.dir, s.projectKey, taskName)
}

func (s *Store) entryPath(taskName string, fp *Fingerprint) string {
	return filepath.Join(s.taskDir(taskName), fp.Hash+".json")
}

// Lookup checks if a cached result exists for the given fingerprint
func (s *Store) Lookup(taskName string, fp *Fingerprint) (*Entry, bool) {
	path := s.entryPath(taskName, fp)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Verify fingerprint matches (sanity check)
	if entry.Fingerprint != fp.Hash {
		return nil, false
	}

	return &entry, true
}

// Store saves a task result to cache
func (s *Store) Store(taskName string, fp *Fingerprint, success bool) error {
	path := s.entryPath(taskName, fp)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	entry := Entry{
		Fingerprint: fp.Hash,
		Success:     success,
		Timestamp:   time.Now(),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Clear removes all cached entries for this project
func (s *Store) Clear() error {
	projectDir := filepath.Join(s.dir, s.projectKey)
	return os.RemoveAll(projectDir)
}

// Stats returns cache statistics for this project
type Stats struct {
	EntryCount int
	TotalSize  int64
	OldestTime time.Time
	NewestTime time.Time
}

// Stats returns statistics about the cache
func (s *Store) Stats() (*Stats, error) {
	projectDir := filepath.Join(s.dir, s.projectKey)

	stats := &Stats{}

	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}

		stats.EntryCount++
		stats.TotalSize += info.Size()

		modTime := info.ModTime()
		if stats.OldestTime.IsZero() || modTime.Before(stats.OldestTime) {
			stats.OldestTime = modTime
		}
		if stats.NewestTime.IsZero() || modTime.After(stats.NewestTime) {
			stats.NewestTime = modTime
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return stats, nil
}
