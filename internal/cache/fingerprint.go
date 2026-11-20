package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// Fingerprint represents a content-based hash of task inputs
type Fingerprint struct {
	Hash string
}

// ComputeFingerprint calculates the fingerprint for a task.
//
// Returns nil (no caching) when:
//   - task.NoCache is true,
//   - the task is a build task: Nix already caches derivations, and only
//     `nix build` can tell whether the derivation changed, or
//   - the task declares no inputs. Without an explicit input set, the
//     fingerprint can't see source-file changes (e.g. switching git branches),
//     so caching would produce stale hits.
func ComputeFingerprint(task config.Task, workDir string) (*Fingerprint, error) {
	if task.NoCache || task.Type == config.TaskTypeBuild {
		return nil, nil
	}
	if len(task.Inputs) == 0 {
		return nil, nil
	}

	h := sha256.New()

	// Hash task definition (excluding description which doesn't affect execution)
	taskData := struct {
		Deps       []string          `json:"deps"`
		Depends    []string          `json:"depends"`
		Commands   []string          `json:"commands"`
		Script     string            `json:"script"`
		Env        map[string]string `json:"env"`
		WorkingDir string            `json:"workingDir"`
	}{
		Deps:       task.Deps,
		Depends:    task.Depends,
		Commands:   task.Commands,
		Script:     task.Script,
		Env:        task.Env,
		WorkingDir: task.WorkingDir,
	}
	taskJSON, err := json.Marshal(taskData)
	if err != nil {
		return nil, err
	}
	h.Write(taskJSON)

	// The shell's drvPath changes whenever any of its packages does, without
	// the cost of resolving each package's store path.
	h.Write([]byte(task.ShellDrvPath))

	// Hash input files if specified
	if len(task.Inputs) > 0 {
		if err := hashInputFiles(h, workDir, task.Inputs); err != nil {
			return nil, err
		}
	}

	return &Fingerprint{
		Hash: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func hashInputFiles(h io.Writer, workDir string, patterns []string) error {
	var files []string

	baseDir := workDir
	if baseDir == "" {
		baseDir = "."
	}
	fsys := os.DirFS(baseDir)

	for _, pattern := range patterns {
		matches, err := doublestar.Glob(fsys, pattern)
		if err != nil {
			return err
		}
		for _, m := range matches {
			files = append(files, filepath.Join(baseDir, m))
		}
	}

	// Sort for determinism
	sort.Strings(files)

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			// Skip files that don't exist or can't be stat'd
			continue
		}
		if info.IsDir() {
			// Skip directories
			continue
		}

		// Write relative path for determinism
		rel := file
		if workDir != "" {
			rel, _ = filepath.Rel(workDir, file)
		}
		h.Write([]byte(rel))

		// Write file content
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		_, _ = io.Copy(h, f)
		f.Close()
	}

	return nil
}
