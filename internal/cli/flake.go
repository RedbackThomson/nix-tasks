package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ResolveFlake makes a local -f path absolute and records the flake root, so
// a relative path is not read as a flake-registry name and tasks behave the
// same from any subdirectory.
func (g *Globals) ResolveFlake() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}
	flake, root, err := resolveFlakeRef(g.Flake, cwd)
	if err != nil {
		return err
	}
	g.Flake = flake
	g.FlakeRoot = root
	return nil
}

// resolveFlakeRef returns the flake reference to pass to nix and the local
// directory containing flake.nix. Non-path references (github:..., path:...,
// registry names) are returned unchanged with an empty root.
func resolveFlakeRef(ref, cwd string) (flake, root string, err error) {
	if !isLocalPath(ref, cwd) {
		return ref, "", nil
	}

	dir := ref
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	root = findFlakeRoot(filepath.Clean(dir))
	return root, root, nil
}

// isLocalPath reports whether ref names a directory rather than a flake URL
// or registry name. A bare word like "nixpkgs" is only a path if it exists.
func isLocalPath(ref, cwd string) bool {
	if strings.Contains(ref, ":") {
		return false
	}
	if filepath.IsAbs(ref) || ref == "." || ref == ".." ||
		strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return true
	}
	info, err := os.Stat(filepath.Join(cwd, ref))
	return err == nil && info.IsDir()
}

// findFlakeRoot returns the directory nix evaluates for the path dir, using
// the same rule as nix so tasks run where the evaluated flake.nix is.
// Symlinks in dir are resolved, and a path to a flake.nix file means its
// directory. If that directory has no flake.nix, the search moves up through
// its parents and gives up at the first directory that contains .git (a file
// or directory), that is on a different filesystem from the starting
// directory, or that is /, whose flake.nix is never found by searching up.
// Where nix would fail, dir is returned unchanged so nix reports the error.
func findFlakeRoot(dir string) string {
	start, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	info, err := os.Lstat(start)
	if err != nil {
		return dir
	}
	if !info.IsDir() {
		if filepath.Base(start) != "flake.nix" {
			return dir
		}
		start = filepath.Dir(start)
	}

	// Checked before the search so that a flake at / is still found.
	if found, _ := lexists(filepath.Join(start, "flake.nix")); found {
		return start
	}
	device, err := deviceOf(start)
	if err != nil {
		return dir
	}
	for d := start; d != string(filepath.Separator); d = filepath.Dir(d) {
		found, ok := lexists(filepath.Join(d, "flake.nix"))
		if !ok {
			return dir
		}
		if found {
			return d
		}
		if isRepo, ok := lexists(filepath.Join(d, ".git")); !ok || isRepo {
			return dir
		}
		if dev, err := deviceOf(d); err != nil || dev != device {
			return dir
		}
	}
	return dir
}

// lexists reports whether path exists without following a final symlink.
// ok is false when that cannot be determined.
func lexists(path string) (exists, ok bool) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return false, true
	}
	return false, false
}

// deviceOf returns the ID of the filesystem holding path. Tests replace it to
// simulate a mount point.
var deviceOf = func(path string) (uint64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("no device for %s", path)
	}
	return uint64(stat.Dev), nil
}
