package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

// devEnvs resolves the environment of each task's shell for one run. Each
// distinct shell derivation is realised at most once per run and, across
// runs, at most once per drvPath: a drvPath fully determines its
// environment, so the result is cached on disk under that key.
type devEnvs struct {
	nix    Nix
	system string
	dir    string

	// pending are the run's tasks whose drvPath is looked up together the
	// first time any of them needs it.
	pending []string
	drvOnce sync.Once
	drvs    map[string]string
	drvErr  error

	mu   sync.Mutex
	envs map[string]*devEnvEntry
}

type devEnvEntry struct {
	once sync.Once
	env  *nix.DevEnv
	err  error
}

func newDevEnvs(n Nix, dir string, pending []string) *devEnvs {
	return &devEnvs{
		nix:     n,
		system:  nix.CurrentSystem(),
		dir:     dir,
		pending: pending,
		envs:    make(map[string]*devEnvEntry),
	}
}

// drvPath returns the drvPath of the named task's shell.
func (d *devEnvs) drvPath(ctx context.Context, name string) (string, error) {
	d.drvOnce.Do(func() {
		start := time.Now()
		d.drvs, d.drvErr = d.nix.ShellDrvPaths(ctx, d.system, d.pending)
		slog.Debug("evaluated task shells", "tasks", len(d.pending), "duration", time.Since(start), "error", d.drvErr)
	})
	if drv := d.drvs[name]; drv != "" {
		return drv, nil
	}
	// One broken shell fails the batch; evaluating this task's shell alone
	// keeps the failure on the tasks that use it.
	drvs, err := d.nix.ShellDrvPaths(ctx, d.system, []string{name})
	if err != nil {
		return "", fmt.Errorf("failed to evaluate the task's shell: %w", err)
	}
	return drvs[name], nil
}

// env returns the environment of the shell derivation drvPath. Output from
// realising the shell goes to progress.
func (d *devEnvs) env(ctx context.Context, drvPath string, progress io.Writer) (*nix.DevEnv, error) {
	d.mu.Lock()
	entry, ok := d.envs[drvPath]
	if !ok {
		entry = &devEnvEntry{}
		d.envs[drvPath] = entry
	}
	d.mu.Unlock()

	entry.once.Do(func() {
		entry.env, entry.err = d.load(ctx, drvPath, progress)
	})
	return entry.env, entry.err
}

func (d *devEnvs) load(ctx context.Context, drvPath string, progress io.Writer) (*nix.DevEnv, error) {
	hash, _, _ := strings.Cut(filepath.Base(drvPath), "-")
	jsonPath := filepath.Join(d.dir, hash+".json")
	profile := filepath.Join(d.dir, hash+"-profile")

	// The profile resolving proves the environment's store paths survived
	// garbage collection, which the JSON alone cannot.
	if _, err := os.Stat(profile); err == nil {
		if data, err := os.ReadFile(jsonPath); err == nil {
			if env, err := nix.ParseDevEnv(data); err == nil {
				slog.Debug("dev env cache hit", "drv", drvPath)
				return env, nil
			}
		}
	}

	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create dev env cache: %w", err)
	}

	cmd := d.nix.PrintDevEnvCmd(ctx, drvPath, profile)
	isolateProcessGroup(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(progress, &stderr)

	start := time.Now()
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nix print-dev-env %s failed: %w\n%s", drvPath, err, strings.TrimSpace(stderr.String()))
	}
	slog.Debug("realised task shell", "drv", drvPath, "duration", time.Since(start))

	env, err := nix.ParseDevEnv(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(jsonPath, stdout.Bytes()); err != nil {
		slog.Debug("failed to cache dev env", "drv", drvPath, "error", err)
	}
	return env, nil
}

// writeFileAtomic writes data to path so concurrent nix-tasks runs never
// read a partial file.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return nil
}

// lookPath finds an executable in pathList, the way exec.LookPath does for
// the current process's PATH.
func lookPath(file, pathList string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, file)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s not found in the task's PATH", file)
}
