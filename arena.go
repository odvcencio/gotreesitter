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

	// Default capacities for slice backing storage used by reduce actions.
	// Full parses allocate many more parent-child edges than incremental edits.
	incrementalChildSliceCap = 2 * 1024
	fullChildSliceCap        = 32 * 1024
	incrementalFieldSliceCap = 2 * 1024
	fullFieldSliceCap        = 32 * 1024
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

	childSlabs      []childSliceSlab
	fieldSlabs      []fieldSliceSlab
	childSlabCursor int
	fieldSlabCursor int
}

type childSliceSlab struct {
	data []*Node
	used int
}

type fieldSliceSlab struct {
	data []FieldID
	used int
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
	childCap := fullChildSliceCap
	fieldCap := fullFieldSliceCap
	if class == arenaClassIncremental {
		childCap = incrementalChildSliceCap
		fieldCap = incrementalFieldSliceCap
	}
	return &nodeArena{
		class:      class,
		nodes:      make([]Node, nodeCapacityForBytes(slabBytes)),
		childSlabs: []childSliceSlab{{data: make([]*Node, childCap)}},
		fieldSlabs: []fieldSliceSlab{{data: make([]FieldID, fieldCap)}},
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

	for i := range a.childSlabs {
		slab := &a.childSlabs[i]
		for j := 0; j < slab.used; j++ {
			slab.data[j] = nil
		}
		slab.used = 0
	}
	for i := range a.fieldSlabs {
		a.fieldSlabs[i].used = 0
	}
	a.childSlabCursor = 0
	a.fieldSlabCursor = 0
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

func (a *nodeArena) allocNodeSlice(n int) []*Node {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]*Node, n)
	}

	if len(a.childSlabs) == 0 {
		a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, defaultChildSliceCap(a.class))})
		a.childSlabCursor = 0
	}
	if a.childSlabCursor < 0 || a.childSlabCursor >= len(a.childSlabs) {
		a.childSlabCursor = 0
	}

	for i := a.childSlabCursor; ; i++ {
		if i >= len(a.childSlabs) {
			capacity := defaultChildSliceCap(a.class)
			if n > capacity {
				capacity = n
			}
			a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, capacity)})
		}

		slab := &a.childSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.childSlabCursor = i
		return slab.data[start:slab.used]
	}
}

func (a *nodeArena) allocFieldIDSlice(n int) []FieldID {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]FieldID, n)
	}

	if len(a.fieldSlabs) == 0 {
		a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, defaultFieldSliceCap(a.class))})
		a.fieldSlabCursor = 0
	}
	if a.fieldSlabCursor < 0 || a.fieldSlabCursor >= len(a.fieldSlabs) {
		a.fieldSlabCursor = 0
	}

	for i := a.fieldSlabCursor; ; i++ {
		if i >= len(a.fieldSlabs) {
			capacity := defaultFieldSliceCap(a.class)
			if n > capacity {
				capacity = n
			}
			a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, capacity)})
		}

		slab := &a.fieldSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSlabCursor = i
		out := slab.data[start:slab.used]
		clear(out)
		return out
	}
}

func defaultChildSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalChildSliceCap
	}
	return fullChildSliceCap
}

func defaultFieldSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalFieldSliceCap
	}
	return fullFieldSliceCap
}
