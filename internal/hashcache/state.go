// Package hashcache bounds pending structural-hash depth without changing cache size.
package hashcache

import (
	"encoding/binary"
	"unsafe"
)

type State uint32

const (
	Complete        State = 1 << 31
	MaxPendingDepth       = 64
	padding               = int(unsafe.Offsetof(struct {
		Ref  uint32
		Hash uint64
	}{}.Hash)) - 4
	LazyDepthSupported = padding != 0
)

// Metadata occupies only padding that the original ref/hash entry already had.
// Architectures without that padding preserve eager hashing and entry size.
type Metadata [padding]byte

func Encode(s State) (m Metadata) {
	if padding != 0 {
		binary.LittleEndian.PutUint32(m[:], uint32(s))
	}
	return m
}

func (m Metadata) State() State {
	if padding == 0 {
		return Complete
	}
	return State(binary.LittleEndian.Uint32(m[:]))
}

func (s State) Depth() State {
	if s&Complete != 0 {
		return 0
	}
	return s
}

func (s State) IncludeChild(child State) State {
	return max(s, child.Depth()+1)
}

func (s State) NeedsHash() bool { return padding == 0 || s >= MaxPendingDepth }
