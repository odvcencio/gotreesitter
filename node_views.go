package gotreesitter

import "sync"

// Detached view metadata is bounded independently of arena and parser pools.
// Released entries contain no node, arena, source, or producing-core pointers.
var nodeViewBackingPool struct {
	sync.Mutex
	free []nodeViewBacking
}

const maxRetainedNodeViewBackingBytes = 4 << 20

// Views share the negative-index dispatch used by final-child sidecars. The
// arena's views field distinguishes them from ordinary sidecar zero, which also
// uses -2. Ordinary parent links keep their existing fast path.
const compactViewChildIndex = -2

func takeNodeViewBacking() nodeViewBacking {
	nodeViewBackingPool.Lock()
	defer nodeViewBackingPool.Unlock()
	n := len(nodeViewBackingPool.free)
	if n == 0 {
		return nil
	}
	v := nodeViewBackingPool.free[n-1]
	nodeViewBackingPool.free[n-1] = nil
	nodeViewBackingPool.free = nodeViewBackingPool.free[:n-1]
	return v
}

func recycleNodeViewBacking(v nodeViewBacking) {
	if v.retainedBytes() > maxRetainedNodeViewBackingBytes {
		return
	}
	nodeViewBackingPool.Lock()
	defer nodeViewBackingPool.Unlock()
	if len(nodeViewBackingPool.free) < 4 {
		nodeViewBackingPool.free = append(nodeViewBackingPool.free, v)
	}
}

func drainNodeViewBackings() {
	nodeViewBackingPool.Lock()
	nodeViewBackingPool.free = nil
	nodeViewBackingPool.Unlock()
}

// Views begin with a small header slab. They retain the ordinary full-tree
// growth and eviction limits once callers actually visit the whole tree.
// A separate pool avoids inheriting a large eager parse's unused reserve.
var nodeViewArenaPool = nodeArenaPool{class: arenaClassFull, maxSize: 4}

func acquireNodeViewArena() *nodeArena {
	p := &nodeViewArenaPool
	p.mu.Lock()
	var arena *nodeArena
	if n := len(p.free); n != 0 {
		arena = p.free[n-1]
		p.free = p.free[:n-1]
	}
	p.mu.Unlock()
	if arena == nil {
		arena = &nodeArena{class: arenaClassFull, viewArena: true, nodes: make([]Node, minArenaNodeCap)}
		arena.recomputeAllocatedBytes()
		if arenaProfileEnabled {
			arenaProfileData.FullNew++
		}
	}
	if arenaProfileEnabled {
		arenaProfileData.FullAcquire++
	}
	arena.refs.Store(1)
	arena.viewNodeLimit = 0
	arena.breakdownEnabled = arenaBreakdownEnabled.Load()
	arena.clearBudget()
	arena.audit = nil
	return arena
}

// A mutating caller commits to visiting the whole tree. Reserve only the
// missing headers, in bounded slabs, without moving pointers already returned.
func reserveNodeViewMaterialization(a *nodeArena, count int) {
	remaining := count - len(a.nodes)
	for i := range a.nodeSlabs {
		remaining -= len(a.nodeSlabs[i].data)
	}
	ceiling := maxOverflowSlabGrowthBytes / int(nodeBytesForCap(1))
	for remaining > 0 {
		capacity := min(remaining, ceiling)
		a.nodeSlabs = append(a.nodeSlabs, nodeSlab{data: make([]Node, capacity)})
		a.allocatedBytes += nodeBytesForCap(capacity)
		remaining -= capacity
	}
}

// nodeViewBacking is the adapter boundary between indexed syntax and the
// public pointer API. Reading creates stable views; mutation first restores
// the complete arena representation, including its reuse receipts.
type nodeViewBacking interface {
	childCount(*Node) int
	namedChildCount(*Node) int
	namedChild(*Node, int) *Node
	child(*Node, int) *Node
	children(*Node) []*Node
	parent(*Node) (*Node, int, bool)
	supertype(*Node) uint32
	executeQuery(*Query, *Node, *Language, []byte, []QueryMatch) ([]QueryMatch, bool)
	equalTree(*Node, *Node, *Language) bool
	copyTree(*Tree) *Tree
	retainedBytes() uint64
	materialize()
	release()
}

func nodeViews(n *Node) nodeViewBacking {
	if n == nil || n.childIndex != compactViewChildIndex || n.ownerArena == nil {
		return nil
	}
	return n.ownerArena.views
}

func materializeNodeViews(n *Node) {
	if views := nodeViews(n); views != nil {
		views.materialize()
	}
}
