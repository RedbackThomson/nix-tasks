package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFlakeRef(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(base, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(p string) {
		mkdir(filepath.Dir(p))
		if err := os.WriteFile(filepath.Join(base, p), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	repo := filepath.Join(base, "repo")
	demo := filepath.Join(repo, "examples", "demo")

	// base/repo is a git repo with a flake at its root and a nested flake
	// in examples/demo; base/noflake is a git repo without any flake.
	mkdir("repo/.git")
	touch("repo/flake.nix")
	mkdir("repo/src/pkg")
	mkdir("repo/examples/demo/sub")
	touch("repo/examples/demo/flake.nix")
	mkdir("noflake/.git")
	mkdir("noflake/sub")
	// base/novcs has a flake and no repository; base/worktree's .git is a
	// file, as in a git worktree; base/outer/jj is a jj workspace that is not
	// colocated with git, so it has no .git.
	touch("novcs/flake.nix")
	mkdir("novcs/a/b")
	mkdir("worktree/sub")
	touch("worktree/.git")
	touch("outer/flake.nix")
	mkdir("outer/jj/.jj")
	mkdir("outer/jj/sub")
	mkdir("plain/sub")
	if err := os.Symlink(demo, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		ref       string
		cwd       string
		wantFlake string
		wantRoot  string
	}{
		{"dot at flake root", ".", repo, repo, repo},
		{"dot from subdirectory walks up", ".", filepath.Join(repo, "src", "pkg"), repo, repo},
		{"nearest flake wins", ".", filepath.Join(demo, "sub"), demo, demo},
		{"bare relative path", "examples/demo", repo, demo, demo},
		{"dot-relative path", "../..", filepath.Join(repo, "src", "pkg"), repo, repo},
		{"absolute path", demo, base, demo, demo},
		{"no flake stops at git root", ".", filepath.Join(base, "noflake", "sub"), filepath.Join(base, "noflake", "sub"), filepath.Join(base, "noflake", "sub")},
		{"git file stops the search", ".", filepath.Join(base, "worktree", "sub"), filepath.Join(base, "worktree", "sub"), filepath.Join(base, "worktree", "sub")},
		{"no repository walks up", ".", filepath.Join(base, "novcs", "a", "b"), filepath.Join(base, "novcs"), filepath.Join(base, "novcs")},
		{"jj workspace without .git is not a boundary", ".", filepath.Join(base, "outer", "jj", "sub"), filepath.Join(base, "outer"), filepath.Join(base, "outer")},
		{"no flake anywhere is left to nix", ".", filepath.Join(base, "plain", "sub"), filepath.Join(base, "plain", "sub"), filepath.Join(base, "plain", "sub")},
		{"flake.nix file means its directory", "./flake.nix", demo, demo, demo},
		{"symlinks are resolved", "link", base, demo, demo},
		{"missing path is left to nix", "./missing", repo, filepath.Join(repo, "missing"), filepath.Join(repo, "missing")},
		{"github ref untouched", "github:owner/repo", repo, "github:owner/repo", ""},
		{"path: ref untouched", "path:./examples/demo", repo, "path:./examples/demo", ""},
		{"registry name untouched", "nixpkgs", repo, "nixpkgs", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flake, root, err := resolveFlakeRef(tt.ref, tt.cwd)
			if err != nil {
				t.Fatalf("resolveFlakeRef() error = %v", err)
			}
			if flake != tt.wantFlake {
				t.Errorf("flake = %q, want %q", flake, tt.wantFlake)
			}
			if root != tt.wantRoot {
				t.Errorf("root = %q, want %q", root, tt.wantRoot)
			}
		})
	}
}

func TestFindFlakeRoot_StopsAtFilesystemBoundary(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(base, "mid")
	mount := filepath.Join(mid, "mnt")
	start := filepath.Join(mount, "sub")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "flake.nix"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// Everything under mnt is on device 2 and everything else on device 1,
	// as if a filesystem were mounted at mnt.
	orig := deviceOf
	t.Cleanup(func() { deviceOf = orig })
	deviceOf = func(path string) (uint64, error) {
		if path == mount || strings.HasPrefix(path, mount+string(filepath.Separator)) {
			return 2, nil
		}
		return 1, nil
	}

	if got := findFlakeRoot(start); got != start {
		t.Errorf("flake beyond the boundary: findFlakeRoot() = %q, want %q (left to nix)", got, start)
	}

	// A directory's flake.nix is checked before its filesystem, so a flake
	// in the first directory past the mount point is still found.
	if err := os.WriteFile(filepath.Join(mid, "flake.nix"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findFlakeRoot(start); got != mid {
		t.Errorf("flake just past the boundary: findFlakeRoot() = %q, want %q", got, mid)
	}
}
