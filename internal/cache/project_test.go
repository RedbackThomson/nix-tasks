package cache

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// jjRepo lays out a jj repository whose first workspace is main, with two
// more workspaces pointing at its repo directory, one by a relative path and
// one by an absolute path, as different jj versions write them.
func jjRepo(t *testing.T) (main, relative, absolute, repo string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main = filepath.Join(base, "main")
	repo = filepath.Join(main, ".jj", "repo")
	mkdir(t, filepath.Join(repo, "store"))

	relative = filepath.Join(base, "work", "feature")
	writeFile(t, filepath.Join(relative, ".jj", "repo"), "../../../main/.jj/repo")

	absolute = filepath.Join(base, "other")
	writeFile(t, filepath.Join(absolute, ".jj", "repo"), repo+"\n")
	return main, relative, absolute, repo
}

func TestJJRepoDir_ResolvesWorkspacePointers(t *testing.T) {
	main, relative, absolute, repo := jjRepo(t)
	for _, dir := range []string{main, relative, absolute, filepath.Join(relative, "sub", "dir")} {
		mkdir(t, dir)
		if got := jjRepoDir(dir); got != repo {
			t.Errorf("jjRepoDir(%s) = %q, want %q", dir, got, repo)
		}
	}
	if got := jjRepoDir(t.TempDir()); got != "" {
		t.Errorf("outside a jj repository: %q", got)
	}
}

func TestProjectKey_JJWorkspacesShareKey(t *testing.T) {
	main, relative, absolute, repo := jjRepo(t)
	ctx := context.Background()
	if got := projectSource(ctx, relative); got != "jjrepo:"+repo {
		t.Fatalf("source = %q", got)
	}
	want := ProjectKey(ctx, main)
	for _, dir := range []string{relative, absolute} {
		if got := ProjectKey(ctx, dir); got != want {
			t.Errorf("ProjectKey(%s) = %s, want %s", dir, got, want)
		}
	}
	if other := ProjectKey(ctx, t.TempDir()); other == want {
		t.Error("an unrelated directory shares the jj repository's key")
	}
}

func TestProjectKey_JJGitBackendOrigin(t *testing.T) {
	_, relative, _, repo := jjRepo(t)
	writeFile(t, filepath.Join(repo, "store", "git_target"), "git")
	writeFile(t, filepath.Join(repo, "store", "git", "config"), `[core]
	bare = true
[remote "upstream"]
	url = https://example.com/fork.git
[remote "origin"]
	url = git@github.com:org/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`)
	if got := projectSource(context.Background(), relative); got != "origin:github.com/org/repo" {
		t.Errorf("source = %q, want the origin shared with git clones", got)
	}
}

func TestProjectKey_FallsBackToPath(t *testing.T) {
	dir := t.TempDir()
	if got := projectSource(context.Background(), dir); got != "path:"+dir {
		t.Errorf("source = %q", got)
	}
}

// testGit runs git in dir, which must be inside the test's temporary
// directory, with the same environment stripping as production plus no
// global or system config, so no inherited GIT_DIR or config can point it
// at another repository.
func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.EvalSymlinks(dir)
	if err != nil || !strings.HasPrefix(abs, tmp+string(filepath.Separator)) {
		t.Fatalf("testGit: %s is not inside %s", dir, tmp)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	cmd := exec.Command("git", append([]string{"-C", abs}, args...)...)
	cmd.Env = append(GitEnv(os.Environ()), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitEnv_InheritedRepositoryIsNeverTouched(t *testing.T) {
	sentinel := t.TempDir()
	testGit(t, sentinel, "init", "-q")
	testGit(t, sentinel, "config", "remote.origin.url", "https://example.com/sentinel.git")
	sentinelConfig := filepath.Join(sentinel, ".git", "config")
	before, err := os.ReadFile(sentinelConfig)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(sentinel, ".git"))
	t.Setenv("GIT_WORK_TREE", sentinel)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(sentinel, ".git", "index"))
	t.Setenv("GIT_CONFIG", sentinelConfig)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")

	other := t.TempDir()
	testGit(t, other, "init", "-q")
	testGit(t, other, "config", "remote.origin.url", "https://example.com/other.git")
	if got := projectSource(context.Background(), other); got != "origin:example.com/other" {
		t.Errorf("source = %q; the inherited GIT_DIR decided the repository", got)
	}
	unrelated := t.TempDir()
	if got := projectSource(context.Background(), unrelated); got != "path:"+unrelated {
		t.Errorf("source = %q for a directory outside any repository", got)
	}

	after, err := os.ReadFile(sentinelConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the repository named by GIT_DIR was modified:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestGitEnv_StripsRepositoryAndConfigVariables(t *testing.T) {
	got := GitEnv([]string{"PATH=/bin", "GIT_DIR=/x", "GIT_WORK_TREE=/x", "GIT_CONFIG_GLOBAL=/x", "GIT_CONFIG_KEY_0=a", "GIT_CEILING_DIRECTORIES=/", "GIT_TERMINAL_PROMPT=0"})
	want := []string{"PATH=/bin", "GIT_TERMINAL_PROMPT=0"}
	if !slices.Equal(got, want) {
		t.Errorf("GitEnv = %v, want %v", got, want)
	}
}
