package cache

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func paths(files []FileEntry) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestHashInputs_Selection(t *testing.T) {
	f := newFixture(t)
	f.write(".git/config", "x")
	f.write("apis/.hidden/x.go", "x")
	f.write(".golangci.yml", "x")
	f.write("vendor/dep/dep.go", "x")
	if err := os.Symlink(filepath.Join(f.root, "vendor"), filepath.Join(f.root, "apis/linked")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		inputs []string
		want   []string
	}{
		{"excludes and outputs", []string{"apis/**/*.go", "!apis/**/testdata/**"}, []string{"apis/v1/types.go"}},
		{"hidden directories are not walked by wildcards", []string{"**/*.go", "!vendor/**"}, []string{"apis/v1/testdata/fixture.go", "apis/v1/types.go"}},
		{"explicit hidden names match", []string{".golangci.yml", ".git/config"}, []string{".git/config", ".golangci.yml"}},
		{"symlinked directories are not traversed", []string{"apis/**"}, []string{"apis/linked", "apis/v1/testdata/fixture.go", "apis/v1/types.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HashInputs(f.root, &config.TaskCache{Inputs: tt.inputs, Outputs: []string{"apis/**/zz_generated.*.go"}})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(paths(got), tt.want) {
				t.Errorf("got %v, want %v", paths(got), tt.want)
			}
		})
	}
}

func TestHashInputs_RootIsFlakeRootRegardlessOfWorkingDir(t *testing.T) {
	f := newFixture(t)
	t.Chdir(filepath.Join(f.root, "apis"))
	f.req.Task.WorkingDir = "apis"
	got, err := HashInputs(f.root, &config.TaskCache{Inputs: []string{"go.mod"}, Outputs: []string{}})
	if err != nil || !slices.Equal(paths(got), []string{"go.mod"}) {
		t.Fatalf("got %v, %v", paths(got), err)
	}
}

func TestHashInputs_PatternMatchingNothing(t *testing.T) {
	f := newFixture(t)
	for _, tt := range []struct {
		pattern string
		dir     bool
	}{{"apis/**/*.proto", false}, {"apis/v1", true}} {
		_, err := HashInputs(f.root, &config.TaskCache{Inputs: []string{"go.mod", tt.pattern}, Outputs: []string{}})
		var noMatch *NoMatchError
		if !errors.As(err, &noMatch) || noMatch.Pattern != tt.pattern || noMatch.Directory != tt.dir {
			t.Errorf("%s: got %v", tt.pattern, err)
		}
	}
}

func TestHashOutputs_LenientAllowsMissing(t *testing.T) {
	f := newFixture(t)
	c := &config.TaskCache{Outputs: []string{"gen/*.go"}}
	if _, err := HashOutputs(f.root, c, false); err == nil {
		t.Error("strict output hashing should fail on a pattern matching nothing")
	}
	got, err := HashOutputs(f.root, c, true)
	if err != nil || len(got) != 0 {
		t.Errorf("lenient: %v, %v", got, err)
	}
}

func TestHashInputs_UnreadableFileFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permissions are not enforced")
	}
	f := newFixture(t)
	path := filepath.Join(f.root, "go.mod")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := HashInputs(f.root, &config.TaskCache{Inputs: []string{"go.mod"}, Outputs: []string{}}); err == nil {
		t.Error("an unreadable input should fail")
	}
}
