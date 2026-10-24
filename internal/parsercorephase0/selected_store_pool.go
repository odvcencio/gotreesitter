package parsercorephase0

import (
	"sync"
	"unsafe"
)

// selectedStorePool is the only owner retained by a published result. It must
// never point back to Core, its table adapter, or parser scratch: a syntax
// result can outlive the parser that produced it. One capped backing pair is
// recycled, and sibling results may release concurrently with another parse.
type selectedStorePool struct {
	mu       sync.Mutex
	backing  selectedStoreBacking
	maxBytes uint64
}

// Core operations are serial; only the detached pool is shared with readers.
func (c *Core) selectedStoreOwner() *selectedStorePool {
	if c.selectedPool == nil {
		c.selectedPool = &selectedStorePool{maxBytes: c.limits.MaxSelectedBytes}
	}
	return c.selectedPool
}

func (p *selectedStorePool) take() selectedStoreBacking {
	p.mu.Lock()
	backing := p.backing
	p.backing = selectedStoreBacking{}
	p.mu.Unlock()
	return backing
}

func (p *selectedStorePool) retainedBytes() uint64 {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	bytes := p.backing.retainedBytes() + uint64(unsafe.Sizeof(*p))
	p.mu.Unlock()
	return bytes
}

func (b selectedStoreBacking) retainedBytes() uint64 {
	return selectedStoreRetainedBytesWithSources(cap(b.records), cap(b.children), cap(b.payloadSources))
}

func (p *selectedStorePool) recycle(records []SelectedNodeRecord, children []SelectedNodeID, sources []SubtreeID) {
	if p == nil {
		return
	}
	incoming := selectedStoreBacking{records: records[:0], children: children[:0], payloadSources: sources[:0]}
	bytes := incoming.retainedBytes()
	if bytes > p.maxBytes {
		return
	}
	// All planes contain values, not pointers. Their lengths are reset and
	// every used slot is overwritten by the next builder before publication.
	// Clearing them on both release and acquire repeats an O(tree size) write
	// that contributes nothing to ownership or GC safety.
	p.mu.Lock()
	if bytes >= p.backing.retainedBytes() {
		p.backing = incoming
	}
	p.mu.Unlock()
}
