package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// File kinds recorded for matched files.
const (
	KindFile    = "file"
	KindSymlink = "symlink"
)

// FileEntry is a matched file: its flake-root-relative slash path, its kind
// and the sha256 of its content (of its target, for a symlink).
type FileEntry struct {
	Path string
	Kind string
	Hash string
}

func (f FileEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]string{f.Path, f.Kind, f.Hash})
}

func (f *FileEntry) UnmarshalJSON(data []byte) error {
	var v [3]string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	f.Path, f.Kind, f.Hash = v[0], v[1], v[2]
	return nil
}

// NoMatchError reports a cache pattern that matched no file. It is fatal
// because a typo would otherwise leave the cache blind to a whole class of
// change.
type NoMatchError struct {
	Field   string // "inputs" or "outputs"
	Pattern string
	// Directory is set when the pattern matched only directories, which
	// contribute nothing.
	Directory bool
}

func (e *NoMatchError) Error() string {
	msg := fmt.Sprintf("cache.%s pattern %q matches no files", e.Field, e.Pattern)
	if e.Directory {
		msg += fmt.Sprintf(" (it names a directory; use %q)", strings.TrimSuffix(e.Pattern, "/")+"/**")
	}
	return msg
}

// patternSet is a list of globs relative to the flake root in which entries
// starting with "!" exclude.
type patternSet struct {
	include []includePattern
	exclude []string
}

type includePattern struct {
	glob string
	base string
	// hidden holds the pattern's literal dot-segments. Wildcards never match
	// dot-files or descend into dot-directories, so .git, .jj and .direnv stay
	// out without a special list, but a pattern naming one explicitly still
	// reaches it.
	hidden map[string]bool
}

func newPatternSet(globs []string) patternSet {
	var s patternSet
	for _, g := range globs {
		if ex, ok := strings.CutPrefix(g, "!"); ok {
			s.exclude = append(s.exclude, ex)
			continue
		}
		base, _ := doublestar.SplitPattern(g)
		p := includePattern{glob: g, base: base, hidden: map[string]bool{}}
		for _, seg := range strings.Split(g, "/") {
			if strings.HasPrefix(seg, ".") && !strings.ContainsAny(seg, `*?[{\`) {
				p.hidden[seg] = true
			}
		}
		s.include = append(s.include, p)
	}
	return s
}

// allows reports whether rel's dot-segments are all named by the pattern.
func (p includePattern) allows(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") && !p.hidden[seg] {
			return false
		}
	}
	return true
}

func (p includePattern) match(rel string) bool {
	return p.allows(rel) && doublestar.MatchUnvalidated(p.glob, rel)
}

// mayMatchUnder reports whether the pattern could match something inside dir.
func (p includePattern) mayMatchUnder(dir string) bool {
	if !p.allows(dir) {
		return false
	}
	if p.base == "." || p.base == dir {
		return true
	}
	return strings.HasPrefix(p.base, dir+"/") || strings.HasPrefix(dir, p.base+"/")
}

func (s patternSet) excluded(rel string) bool {
	for _, ex := range s.exclude {
		if doublestar.MatchUnvalidated(ex, rel) {
			return true
		}
	}
	return false
}

// matches reports whether rel is selected by s: matched by an include
// pattern and not excluded.
func (s patternSet) matches(rel string) bool {
	for _, p := range s.include {
		if p.match(rel) {
			return !s.excluded(rel)
		}
	}
	return false
}

// prune reports whether nothing inside dir can be selected.
func (s patternSet) prune(dir string) bool {
	for _, ex := range s.exclude {
		if prefix, ok := strings.CutSuffix(ex, "/**"); ok && doublestar.MatchUnvalidated(prefix, dir) {
			return true
		}
	}
	for _, p := range s.include {
		if p.mayMatchUnder(dir) {
			return false
		}
	}
	return true
}

// collect walks root and hashes every file selected by set, minus any
// selected by without, sorted by path. Symlinks are never followed. Unless
// lenient, an include pattern matching no file is a *NoMatchError.
func collect(root, field string, set patternSet, without *patternSet, lenient bool) ([]FileEntry, error) {
	matched := make([]bool, len(set.include))
	matchedDir := make([]bool, len(set.include))
	var files []FileEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			for i, p := range set.include {
				if !matchedDir[i] && p.match(rel) {
					matchedDir[i] = true
				}
			}
			if set.prune(rel) {
				return fs.SkipDir
			}
			return nil
		}

		kind := KindFile
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			kind = KindSymlink
		case !d.Type().IsRegular():
			return nil
		}

		selected := false
		for i, p := range set.include {
			if p.match(rel) {
				matched[i] = true
				selected = true
			}
		}
		if !selected || set.excluded(rel) || (without != nil && without.matches(rel)) {
			return nil
		}

		hash, err := hashFile(path, kind)
		if err != nil {
			return err
		}
		files = append(files, FileEntry{Path: rel, Kind: kind, Hash: hash})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hashing cache.%s: %w", field, err)
	}

	if !lenient {
		for i, p := range set.include {
			if !matched[i] {
				return nil, &NoMatchError{Field: field, Pattern: p.glob, Directory: matchedDir[i]}
			}
		}
	}

	slices.SortFunc(files, func(a, b FileEntry) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

func hashFile(path, kind string) (string, error) {
	h := sha256.New()
	if kind == KindSymlink {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(target))
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
