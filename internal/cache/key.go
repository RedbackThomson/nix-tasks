package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"strconv"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// FormatVersion changes whenever what goes into a key, or how it is encoded,
// changes. Entries of other versions are never read, so old keys simply stop
// matching.
const FormatVersion = 2

// Components are the hex digests a key is built from. Entries record them so
// a miss can be explained by the ones that differ.
type Components struct {
	Definition string `json:"definition"`
	Toolchain  string `json:"toolchain"`
	Env        string `json:"env"`
	Inputs     string `json:"inputs"`
	Upstream   string `json:"upstream"`
}

// Detail is what a key was computed from, kept so a later miss can be
// diffed against it. Environment values appear only as hashes because they
// can be secrets.
type Detail struct {
	System       string            `json:"system"`
	ShellDrvPath string            `json:"shellDrvPath"`
	Env          map[string]string `json:"env,omitempty"`
	Upstream     map[string]string `json:"upstream,omitempty"`
	// UpstreamDrvs holds the derivation path of each direct build-task
	// dependency, whose upstream key is derived from it.
	UpstreamDrvs map[string]string `json:"upstreamDrvs,omitempty"`
	Inputs       []FileEntry       `json:"inputs"`
	Outputs      []FileEntry       `json:"outputs"`
}

// Key identifies one state of a cached task.
type Key struct {
	Hash       string
	Components Components
	Detail     Detail
}

// Request is everything a task's key is computed from.
type Request struct {
	// Root is the flake root every pattern is relative to.
	Root   string
	System string
	Task   config.Task
	// Upstream maps each direct dependency (depends and after targets) to
	// its key; a build task's key comes from BuildKey.
	Upstream map[string]string
	// UpstreamDrvs is the drvPath of each direct build-task dependency,
	// recorded so a miss can say that a derivation changed.
	UpstreamDrvs map[string]string
	// LookupEnv reads the environment; nil means os.LookupEnv.
	LookupEnv func(string) (string, bool)
}

// ComputeKey hashes a cached task's definition, toolchain, declared
// environment, input files and upstream keys. Input files that also match
// an output pattern are left out, so generated files living beside their
// sources do not invalidate their own entry.
func ComputeKey(req Request) (*Key, error) {
	c := req.Task.Cache
	inputs, err := HashInputs(req.Root, c)
	if err != nil {
		return nil, err
	}

	lookup := req.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	k := &Key{Detail: Detail{
		System:       req.System,
		ShellDrvPath: req.Task.ShellDrvPath,
		Env:          map[string]string{},
		Upstream:     req.Upstream,
		UpstreamDrvs: req.UpstreamDrvs,
		Inputs:       inputs,
	}}

	k.Components.Definition = digest(func(e *encoder) {
		t := req.Task
		e.field("commands")
		e.list(t.Commands)
		e.field("script")
		e.optional(t.Script != "", t.Script)
		e.field("env")
		e.strMap(t.Env)
		e.field("workingDir")
		e.optional(t.WorkingDir != "", t.WorkingDir)
		e.field("cache.inputs")
		e.list(c.Inputs)
		e.field("cache.outputs")
		e.list(c.Outputs)
		e.field("cache.env")
		e.list(c.Env)
	})

	k.Components.Toolchain = digest(func(e *encoder) {
		e.field("system")
		e.str(req.System)
		e.field("shellDrvPath")
		e.str(req.Task.ShellDrvPath)
	})

	names := slices.Clone(c.Env)
	slices.Sort(names)
	names = slices.Compact(names)
	k.Components.Env = digest(func(e *encoder) {
		e.uvarint(uint64(len(names)))
		for _, name := range names {
			value, set := lookup(name)
			e.str(name)
			e.optional(set, value)
			k.Detail.Env[name] = envFingerprint(value, set)
		}
	})

	k.Components.Inputs = filesDigest(inputs)

	k.Components.Upstream = digest(func(e *encoder) {
		e.strMap(req.Upstream)
	})

	k.Hash = digest(func(e *encoder) {
		e.frame("nix-tasks-cache", []byte(strconv.Itoa(FormatVersion)))
		for _, part := range []struct{ tag, hex string }{
			{"definition", k.Components.Definition},
			{"toolchain", k.Components.Toolchain},
			{"env", k.Components.Env},
			{"inputs", k.Components.Inputs},
			{"upstream", k.Components.Upstream},
		} {
			e.frame(part.tag, []byte(part.hex))
		}
	})
	return k, nil
}

// BuildKey is the key a build task publishes to cached tasks downstream: its
// derivation path, which changes whenever anything it builds from does, and
// the keys of its own upstream tasks.
func BuildKey(drvPath string, upstream map[string]string) string {
	return digest(func(e *encoder) {
		e.frame("nix-tasks-build", []byte(strconv.Itoa(FormatVersion)))
		e.field("drvPath")
		e.str(drvPath)
		e.field("upstream")
		e.strMap(upstream)
	})
}

// HashInputs hashes the files a cached task reads. Every include pattern
// must match at least one file.
func HashInputs(root string, c *config.TaskCache) ([]FileEntry, error) {
	outputs := newPatternSet(c.Outputs)
	return collect(root, "inputs", newPatternSet(c.Inputs), &outputs, false)
}

// HashOutputs hashes the files a cached task writes. Unless lenient, every
// include pattern must match at least one file; lookups are lenient because
// missing outputs are simply a miss.
func HashOutputs(root string, c *config.TaskCache, lenient bool) ([]FileEntry, error) {
	return collect(root, "outputs", newPatternSet(c.Outputs), nil, lenient)
}

// OutputDigest is the digest recorded for a set of output files.
func OutputDigest(files []FileEntry) string {
	return filesDigest(files)
}

func filesDigest(files []FileEntry) string {
	return digest(func(e *encoder) {
		e.uvarint(uint64(len(files)))
		for _, f := range files {
			e.str(f.Path)
			e.str(f.Kind)
			e.str(f.Hash)
		}
	})
}

func envFingerprint(value string, set bool) string {
	if !set {
		return "unset"
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digest(write func(*encoder)) string {
	e := &encoder{h: sha256.New()}
	write(e)
	return hex.EncodeToString(e.h.Sum(nil))
}
