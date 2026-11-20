package cache

import (
	"encoding/binary"
	"hash"
	"slices"
)

// encoder writes the canonical byte stream digests are computed over. Every
// value is length-prefixed or counted, so no two different sequences of
// values can produce the same bytes; concatenated strings or encoding/json
// output give no such guarantee.
type encoder struct {
	h       hash.Hash
	scratch [binary.MaxVarintLen64]byte
}

func (e *encoder) uvarint(n uint64) {
	e.h.Write(e.scratch[:binary.PutUvarint(e.scratch[:], n)])
}

func (e *encoder) bytes(b []byte) {
	e.uvarint(uint64(len(b)))
	e.h.Write(b)
}

func (e *encoder) str(s string) {
	e.bytes([]byte(s))
}

// frame writes a tagged value.
func (e *encoder) frame(tag string, value []byte) {
	e.str(tag)
	e.bytes(value)
}

// field writes a tag ahead of a self-delimiting value written by the caller.
func (e *encoder) field(tag string) {
	e.str(tag)
}

func (e *encoder) list(items []string) {
	e.uvarint(uint64(len(items)))
	for _, item := range items {
		e.str(item)
	}
}

func (e *encoder) strMap(m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	e.uvarint(uint64(len(keys)))
	for _, k := range keys {
		e.frame(k, []byte(m[k]))
	}
}

// optional distinguishes an absent value from an empty one.
func (e *encoder) optional(present bool, s string) {
	if !present {
		e.h.Write([]byte{0})
		return
	}
	e.h.Write([]byte{1})
	e.str(s)
}
