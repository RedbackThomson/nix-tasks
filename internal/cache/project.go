package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ProjectKey names the store shared by every checkout of the repository
// containing root: git worktrees, jj workspaces and separate clones. It
// hashes the first available of the normalised origin URL, the git common
// directory (shared by a clone's worktrees), the jj repository (shared by its
// workspaces, or its git backend's origin URL) and root itself.
func ProjectKey(ctx context.Context, root string) string {
	sum := sha256.Sum256([]byte(projectSource(ctx, root)))
	return hex.EncodeToString(sum[:])[:16]
}

func projectSource(ctx context.Context, root string) string {
	if origin := git(ctx, root, "config", "--get", "remote.origin.url"); origin != "" {
		return "origin:" + normalizeRemote(origin)
	}
	if common := git(ctx, root, "rev-parse", "--git-common-dir"); common != "" {
		if !filepath.IsAbs(common) {
			common = filepath.Join(root, common)
		}
		return "gitdir:" + filepath.Clean(common)
	}
	if repo := jjRepoDir(root); repo != "" {
		if origin := jjGitOrigin(repo); origin != "" {
			return "origin:" + normalizeRemote(origin)
		}
		return "jjrepo:" + repo
	}
	return "path:" + root
}

// jjRepoDir returns the repository directory of the jj workspace containing
// dir, or "". It reads the filesystem, so jj need not be installed: the
// workspace that created the repository holds it in .jj/repo, and every other
// workspace's .jj/repo is a file naming that directory, absolute or relative
// to its own .jj directory.
func jjRepoDir(dir string) string {
	for d := dir; ; {
		jjDir := filepath.Join(d, ".jj")
		if info, err := os.Stat(jjDir); err == nil && info.IsDir() {
			return resolveJJRepo(jjDir)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func resolveJJRepo(jjDir string) string {
	repo := filepath.Join(jjDir, "repo")
	info, err := os.Stat(repo)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		data, err := os.ReadFile(repo)
		if err != nil {
			return ""
		}
		target := strings.TrimSpace(string(data))
		if target == "" {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(jjDir, target)
		}
		repo = target
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	return filepath.Clean(repo)
}

// jjGitOrigin returns the origin URL of a jj repository's git backend, so a
// jj workspace shares entries with git clones of the same remote.
func jjGitOrigin(repo string) string {
	store := filepath.Join(repo, "store")
	target, err := os.ReadFile(filepath.Join(store, "git_target"))
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(target))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(store, gitDir)
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	return gitConfigValue(string(data), `remote "origin"`, "url")
}

// gitConfigValue reads key from section in a git config file. It handles
// the plain "key = value" lines git writes for remotes, not includes or
// quoting.
func gitConfigValue(config, section, key string) string {
	in := false
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]") == section
			continue
		}
		if !in {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// gitRepositoryVars choose a repository, or how one is found, regardless of
// -C. git sets some of them for hooks and `rebase --exec`, where they would
// name the wrong repository; variables starting GIT_CONFIG are removed too.
var gitRepositoryVars = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
	"GIT_NAMESPACE", "GIT_PREFIX", "GIT_SHALLOW_FILE", "GIT_GRAFT_FILE",
	"GIT_REPLACE_REF_BASE", "GIT_QUARANTINE_PATH",
}

// GitEnv returns environ without the variables that would make git use a
// repository other than the one found from its -C directory.
func GitEnv(environ []string) []string {
	return slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(gitRepositoryVars, name) || strings.HasPrefix(name, "GIT_CONFIG")
	})
}

func git(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = GitEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// normalizeRemote reduces the spellings of one remote (https, ssh, scp-like,
// with or without .git or credentials) to host/path.
func normalizeRemote(remote string) string {
	r := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(remote), "/"), ".git")
	if strings.Contains(r, "://") {
		if u, err := url.Parse(r); err == nil && u.Host != "" {
			return strings.ToLower(u.Hostname()) + "/" + strings.Trim(u.Path, "/")
		}
		return r
	}
	if host, path, ok := strings.Cut(r, ":"); ok && !strings.Contains(host, "/") {
		if _, h, found := strings.Cut(host, "@"); found {
			host = h
		}
		return strings.ToLower(host) + "/" + strings.Trim(path, "/")
	}
	return r
}

// RemoveLegacy removes the entries an older nix-tasks kept for the flake at
// flakePath, directly under base and keyed by the flake's path.
func RemoveLegacy(base, flakePath string) error {
	abs, err := filepath.Abs(flakePath)
	if err != nil {
		abs = flakePath
	}
	sum := sha256.Sum256([]byte(abs))
	return os.RemoveAll(filepath.Join(base, hex.EncodeToString(sum[:8])))
}
