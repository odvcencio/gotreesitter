package gotreesitter

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	// incrementalArenaSlab is sized for steady-state edits where only a small
	// frontier of nodes is rebuilt.
	incrementalArenaSlab = 16 * 1024
	// fullParseArenaSlab matches the current full-parse node footprint with
	// headroom, while remaining small enough to keep a warm pool.
	fullParseArenaSlab = 2 * 1024 * 1024
	minArenaNodeCap    = 64
)

type arenaClass uint8

const (
	arenaClassIncremental arenaClass = iota
	arenaClassFull
)

// nodeArena is a slab-backed allocator for Node structs.
// It uses ref counting so trees that borrow reused subtrees can keep arena
// memory alive safely until all dependent trees are released.
type nodeArena struct {
	class arenaClass
	nodes []Node
	used  int
	refs  atomic.Int32
}

var (
	incrementalArenaPool = sync.Pool{
		New: func() any {
			return newNodeArena(arenaClassIncremental, incrementalArenaSlab)
		},
	}
	fullArenaPool = sync.Pool{
		New: func() any {
			return newNodeArena(arenaClassFull, fullParseArenaSlab)
		},
	}
)

func nodeCapacityForBytes(slabBytes int) int {
	nodeSize := int(unsafe.Sizeof(Node{}))
	if nodeSize <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / nodeSize
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func newNodeArena(class arenaClass, slabBytes int) *nodeArena {
	return &nodeArena{
		class: class,
		nodes: make([]Node, nodeCapacityForBytes(slabBytes)),
	}
}

func acquireNodeArena(class arenaClass) *nodeArena {
	var a *nodeArena
	switch class {
	case arenaClassIncremental:
		a = incrementalArenaPool.Get().(*nodeArena)
	default:
		a = fullArenaPool.Get().(*nodeArena)
	}
	a.refs.Store(1)
	return a
}

func (a *nodeArena) Retain() {
	if a == nil {
		return
	}
	a.refs.Add(1)
}

func (a *nodeArena) Release() {
	if a == nil {
		return
	}
	if a.refs.Add(-1) != 0 {
		return
	}
	a.reset()
	switch a.class {
	case arenaClassIncremental:
		incrementalArenaPool.Put(a)
	default:
		fullArenaPool.Put(a)
	}
}

func (a *nodeArena) reset() {
	for i := 0; i < a.used; i++ {
		a.nodes[i] = Node{}
	}
	a.used = 0
}

func (a *nodeArena) allocNode() *Node {
	if a == nil {
		return &Node{}
	}
	if a.used < len(a.nodes) {
		n := &a.nodes[a.used]
		a.used++
		*n = Node{}
		return n
	}
	// Fallback when slab is exhausted.
	return &Node{}
}
