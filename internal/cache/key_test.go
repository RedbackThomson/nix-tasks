package cache

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

type fixture struct {
	t    *testing.T
	root string
	req  Request
	env  map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), env: map[string]string{"GOFLAGS": "-mod=mod"}}
	f.write("apis/v1/types.go", "package v1")
	f.write("apis/v1/zz_generated.deepcopy.go", "generated")
	f.write("apis/v1/testdata/fixture.go", "fixture")
	f.write("go.mod", "module x")
	f.req = Request{
		Root:   f.root,
		System: "x86_64-linux",
		Task: config.Task{
			Description: "generate",
			Commands:    []string{"go generate ./..."},
			Env:         map[string]string{"A": "1"},
			After:       []string{"task:x", "task:y"},
			Cache: &config.TaskCache{
				Inputs:  []string{"apis/**/*.go", "go.mod", "!apis/**/testdata/**"},
				Outputs: []string{"apis/**/zz_generated.*.go"},
				Env:     []string{"GOFLAGS", "UNSET_VAR"},
			},
			ShellDrvPath: "/nix/store/a-nix-shell.drv",
		},
		Upstream: map[string]string{"tools": "k1"},
	}
	f.req.LookupEnv = func(name string) (string, bool) {
		v, ok := f.env[name]
		return v, ok
	}
	return f
}

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	path := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) key() *Key {
	f.t.Helper()
	k, err := ComputeKey(f.req)
	if err != nil {
		f.t.Fatalf("ComputeKey: %v", err)
	}
	return k
}

func TestComputeKey(t *testing.T) {
	tests := []struct {
		name      string
		change    func(f *fixture)
		changes   bool
		component func(c Components) string
	}{
		{"commands", func(f *fixture) { f.req.Task.Commands = []string{"go generate ./apis/..."} }, true, def},
		{"script", func(f *fixture) { f.req.Task.Commands = nil; f.req.Task.Script = "go generate ./..." }, true, def},
		{"literal env", func(f *fixture) { f.req.Task.Env = map[string]string{"A": "2"} }, true, def},
		{"workingDir", func(f *fixture) { f.req.Task.WorkingDir = "apis" }, true, def},
		{"input pattern", func(f *fixture) { f.req.Task.Cache.Inputs = append(f.req.Task.Cache.Inputs, "go.*") }, true, def},
		{"output pattern", func(f *fixture) { f.req.Task.Cache.Outputs = []string{"apis/**/zz_generated.deepcopy.go"} }, true, def},
		{"env pattern", func(f *fixture) { f.req.Task.Cache.Env = []string{"GOFLAGS"} }, true, def},
		{"description", func(f *fixture) { f.req.Task.Description = "other" }, false, def},
		{"continueOnError", func(f *fixture) { f.req.Task.ContinueOnError = true }, false, def},
		{"after-hook order", func(f *fixture) { f.req.Task.After = []string{"task:y", "task:x"} }, false, def},
		{"deps names", func(f *fixture) { f.req.Task.Deps = []string{"go"} }, false, def},
		{"shellDrvPath", func(f *fixture) { f.req.Task.ShellDrvPath = "/nix/store/b-nix-shell.drv" }, true, tool},
		{"system", func(f *fixture) { f.req.System = "aarch64-darwin" }, true, tool},
		{"declared env value", func(f *fixture) { f.env["GOFLAGS"] = "-mod=vendor" }, true, envc},
		{"declared env unset vs empty", func(f *fixture) { f.env["UNSET_VAR"] = "" }, true, envc},
		{"undeclared env", func(f *fixture) { f.env["HOME"] = "/elsewhere" }, false, envc},
		{"input content", func(f *fixture) { f.write("apis/v1/types.go", "package v1 // edited") }, true, inp},
		{"file added", func(f *fixture) { f.write("apis/v1/new.go", "package v1") }, true, inp},
		{"file removed", func(f *fixture) {
			_ = os.Remove(filepath.Join(f.root, "apis/v1/types.go"))
			f.write("apis/v1/other.go", "x")
		}, true, inp},
		{"file renamed", func(f *fixture) {
			_ = os.Rename(filepath.Join(f.root, "apis/v1/types.go"), filepath.Join(f.root, "apis/v1/types2.go"))
		}, true, inp},
		{"file matching inputs and outputs", func(f *fixture) { f.write("apis/v1/zz_generated.deepcopy.go", "regenerated") }, false, inp},
		{"excluded file", func(f *fixture) { f.write("apis/v1/testdata/fixture.go", "edited") }, false, inp},
		{"untracked file", func(f *fixture) { f.write("README.md", "docs") }, false, inp},
		{"upstream key", func(f *fixture) { f.req.Upstream = map[string]string{"tools": "k2"} }, true, up},
		{"absolute location", func(f *fixture) {
			moved := filepath.Join(t.TempDir(), "elsewhere")
			if err := os.Rename(f.root, moved); err != nil {
				t.Fatal(err)
			}
			f.root, f.req.Root = moved, moved
		}, false, def},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			before := f.key()
			tt.change(f)
			after := f.key()
			if got := before.Hash != after.Hash; got != tt.changes {
				t.Errorf("key changed = %v, want %v", got, tt.changes)
			}
			if got := tt.component(before.Components) != tt.component(after.Components); got != tt.changes {
				t.Errorf("component changed = %v, want %v", got, tt.changes)
			}
		})
	}
}

func def(c Components) string  { return c.Definition }
func tool(c Components) string { return c.Toolchain }
func envc(c Components) string { return c.Env }
func inp(c Components) string  { return c.Inputs }
func up(c Components) string   { return c.Upstream }

func TestComputeKey_StableAcrossRunsAndWorkingDirectory(t *testing.T) {
	f := newFixture(t)
	f.req.Task.Env = map[string]string{"A": "1", "B": "2", "C": "3", "D": "4"}
	first := f.key()
	t.Chdir(t.TempDir())
	for range 20 {
		if k := f.key(); k.Hash != first.Hash {
			t.Fatal("key differs between computations")
		}
	}
}

func TestComputeKey_SymlinkTargetHashed(t *testing.T) {
	f := newFixture(t)
	f.req.Task.Cache.Inputs = []string{"result"}
	if err := os.Symlink("/nix/store/aaa-tool", filepath.Join(f.root, "result")); err != nil {
		t.Fatal(err)
	}
	before := f.key()
	if got := before.Detail.Inputs; len(got) != 1 || got[0].Kind != KindSymlink {
		t.Fatalf("inputs %v", got)
	}
	_ = os.Remove(filepath.Join(f.root, "result"))
	if err := os.Symlink("/nix/store/bbb-tool", filepath.Join(f.root, "result")); err != nil {
		t.Fatal(err)
	}
	if f.key().Hash == before.Hash {
		t.Error("retargeting a symlink should change the key")
	}
}

func TestComputeKey_RemovedUpstreamChangesKey(t *testing.T) {
	f := newFixture(t)
	f.req.Upstream = nil
	if k := f.key(); k.Components.Upstream == newFixture(t).key().Components.Upstream {
		t.Error("removing an upstream should change the upstream component")
	}
}

func TestEncoder_FramesAreUnambiguous(t *testing.T) {
	enc := func(items ...string) string {
		return digest(func(e *encoder) { e.list(items) })
	}
	if enc("a", "bc") == enc("ab", "c") {
		t.Error("a+bc and ab+c encode the same")
	}
	files := func(path, hash string) string {
		return filesDigest([]FileEntry{{Path: path, Kind: KindFile, Hash: hash}})
	}
	if files("a", "bc") == files("ab", "c") {
		t.Error("path/content boundary is ambiguous")
	}
	opt := func(present bool) string { return digest(func(e *encoder) { e.optional(present, "") }) }
	if opt(true) == opt(false) {
		t.Error("absent and empty optional values encode the same")
	}
	if got := sha256.Size * 2; len(enc("x")) != got {
		t.Errorf("digest length %d", len(enc("x")))
	}
}
