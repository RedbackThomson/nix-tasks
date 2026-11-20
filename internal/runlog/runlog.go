// Package runlog keeps the directory of task output logs for each run.
package runlog

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

// Keep is how many run directories are kept; older ones are removed when a
// run starts.
const Keep = 20

// idLayout sorts lexically in time order, so the newest runs sort last.
const idLayout = "20060102T150405.000Z"

var idPattern = regexp.MustCompile(`^\d{8}T\d{6}\.\d{3}Z-[0-9a-f]{6}$`)

// Root returns the directory holding every run's logs:
// $XDG_STATE_HOME/nix-tasks/runs, or ~/.local/state/nix-tasks/runs.
func Root() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "nix-tasks", "runs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "nix-tasks", "runs"), nil
}

// NewID returns a run ID for a run starting at now: a UTC timestamp, so IDs
// sort by start time, and a random suffix so concurrent runs differ.
func NewID(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return now.UTC().Format(idLayout) + "-" + hex.EncodeToString(b[:]), nil
}

// Start creates the log directory of a run starting at now under root and
// removes all but the newest keep run directories, including the new one.
func Start(root string, now time.Time, keep int) (string, error) {
	id, err := NewID(now)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create run log directory: %w", err)
	}
	if err := Prune(root, keep); err != nil {
		return dir, fmt.Errorf("failed to remove old run logs: %w", err)
	}
	return dir, nil
}

// Prune removes all but the newest keep run directories under root. Entries
// that are not run directories are left alone.
func Prune(root string, keep int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var runs []string
	for _, e := range entries {
		if e.IsDir() && idPattern.MatchString(e.Name()) {
			runs = append(runs, e.Name())
		}
	}
	if len(runs) <= keep {
		return nil
	}
	slices.Sort(runs)
	var errs []error
	for _, name := range runs[:len(runs)-keep] {
		errs = append(errs, os.RemoveAll(filepath.Join(root, name)))
	}
	return errors.Join(errs...)
}
