package gotreesitter

import (
	"github.com/odvcencio/gotreesitter/internal/incr"
	"slices"
	"unsafe"
)

// Sort immutable receipt coordinates by start byte. Each midpoint stores the
// largest dependency end in its implicit binary subtree.
type compactReuseDependencyIndexEntry struct {
	node   *Node
	end    uint64
	maxEnd uint64
	start  uint32
	live   bool
}

// A receipt stores the examined extent beyond the node's end. Map membership
// distinguishes an authenticated zero extent from a node without a receipt.
func (arena *nodeArena) denseDependencyStore(node *Node) (*incr.DenseReceipts, int, bool) {
	if index, ok := nodeIndexInStorage(node, arena.nodes); ok {
		return &arena.compactReuseDependencies, index, true
	}
	offset := len(arena.nodes)
	for i := range arena.nodeSlabs {
		slab := &arena.nodeSlabs[i]
		if index, ok := nodeIndexInStorage(node, slab.data); ok {
			return &arena.compactReuseDependencies, offset + index, true
		}
		offset += len(slab.data)
	}
	return nil, 0, false
}
func (arena *nodeArena) denseDependencyCount() int { return arena.compactReuseDependencies.Count() }
func (arena *nodeArena) eachDenseDependency(visit func(*Node, incr.DenseReceipt)) {
	arena.compactReuseDependencies.Each(func(index int, receipt incr.DenseReceipt) {
		if index < len(arena.nodes) {
			visit(&arena.nodes[index], receipt)
			return
		}
		index -= len(arena.nodes)
		for i := range arena.nodeSlabs {
			slab := &arena.nodeSlabs[i]
			if index < len(slab.data) {
				visit(&slab.data[index], receipt)
				return
			}
			index -= len(slab.data)
		}
	})
}
func (arena *nodeArena) clearDenseDependency(node *Node) {
	if store, index, ok := arena.denseDependencyStore(node); ok {
		store.Clear(index)
	}
}
func setCompactReuseDependency(node *Node, lookaheadBytes uint32) bool {
	if node == nil || node.ownerArena == nil {
		return false
	}
	arena := node.ownerArena
	arena.compactReuseDependencyMu.Lock()
	defer arena.compactReuseDependencyMu.Unlock()
	store, index, ok := arena.denseDependencyStore(node)
	if !ok {
		return false
	}
	prior := store.Lookup(index)
	if prior != nil && prior.Present && prior.Start == node.startByte && prior.End == node.endByte && prior.Column == node.startPoint.Column {
		lookaheadBytes = maxUint32(lookaheadBytes, prior.Lookahead)
	}
	if store.Ensure(index, func(cost int64) bool {
		used := max(int64(0), arena.allocatedBytes-arena.budgetBaselineBytes)
		if arena.budgetBytes > 0 && (used >= arena.budgetBytes || cost > arena.budgetBytes-used) {
			return false
		}
		arena.allocatedBytes += cost
		return true
	}) == nil {
		return false
	}
	store.Set(index, incr.DenseReceipt{Start: node.startByte, End: node.endByte, Lookahead: lookaheadBytes, Column: node.startPoint.Column})
	arena.compactReuseDependencyIndexed = false
	return true
}
func compactReuseDependencyForNode(node *Node) (uint32, bool) {
	if node == nil || node.ownerArena == nil {
		return 0, false
	}
	arena := node.ownerArena
	arena.compactReuseDependencyMu.RLock()
	defer arena.compactReuseDependencyMu.RUnlock()
	store, index, ok := arena.denseDependencyStore(node)
	if !ok {
		return 0, false
	}
	receipt := store.Lookup(index)
	if receipt == nil || !receipt.Present {
		return 0, false
	}
	return receipt.Lookahead, receipt.Start == node.startByte && receipt.End == node.endByte && receipt.Column == node.startPoint.Column
}
func clearCompactReuseDependency(node *Node) {
	if node != nil && node.ownerArena != nil {
		arena := node.ownerArena
		arena.compactReuseDependencyMu.Lock()
		arena.clearDenseDependency(node)
		arena.compactReuseDependencyIndexed = false
		arena.compactReuseDependencyMu.Unlock()
	}
}
func (arena *nodeArena) compactReuseDependencyBytesAllocated() int64 {
	if arena == nil {
		return 0
	}
	bytes := int64(cap(arena.compactReuseDependencyIndex))*int64(unsafe.Sizeof(compactReuseDependencyIndexEntry{})) + arena.compactReuseDependencies.Bytes()
	return bytes
}

func copyCompactReuseDependency(dst, src *Node) {
	extent, ok := compactReuseDependencyForNode(src)
	clearCompactReuseDependency(dst)
	// A stateless external scanner can still depend on its starting column.
	if ok && dst != nil && src.startPoint.Column == dst.startPoint.Column {
		setCompactReuseDependency(dst, extent)
	}
}

// Invalidate before coordinates change. The normal edit walk can skip nodes
// whose text precedes the edit, although their lexer inspected the edited bytes.
func (arena *nodeArena) editCompactReuseDependencies(edit InputEdit) {
	if arena == nil || inputEditIsNoop(edit) {
		return
	}
	arena.compactReuseDependencyMu.Lock()
	defer arena.compactReuseDependencyMu.Unlock()
	if arena.denseDependencyCount() == 0 {
		return
	}
	if arena.indexCompactReuseDependencies() {
		arena.invalidateIndexedCompactReuseDependencies(edit, 0, len(arena.compactReuseDependencyIndex))
		return
	}
	// Keep exact invalidation when the budget cannot fund an index.
	arena.eachDenseDependency(func(node *Node, receipt incr.DenseReceipt) {
		end := uint64(receipt.End) + uint64(receipt.Lookahead)
		if compactReuseDependencyIntersectsEdit(receipt.Start, end, edit) {
			arena.clearDenseDependency(node)
		}
	})
}

func compactReuseDependencyIntersectsEdit(start uint32, end uint64, edit InputEdit) bool {
	// An arena can supply several live trees. Never inspect or relocate another
	// tree's node coordinates during an edit.
	if edit.OldEndByte <= start &&
		(edit.NewEndByte != edit.OldEndByte || edit.NewEndPoint != edit.OldEndPoint) {
		return true
	}
	return uint64(edit.StartByte) < end && (edit.OldEndByte > start ||
		(edit.StartByte == edit.OldEndByte && edit.StartByte >= start))
}

// Build only after publication changes the receipts. Reuse the backing storage
// across edits, and charge growth before allocating it.
func (arena *nodeArena) indexCompactReuseDependencies() bool {
	if arena.compactReuseDependencyIndexed {
		return true
	}
	count := arena.denseDependencyCount()
	if count > cap(arena.compactReuseDependencyIndex) {
		cost := int64(count-cap(arena.compactReuseDependencyIndex)) * int64(unsafe.Sizeof(compactReuseDependencyIndexEntry{}))
		used := max(int64(0), arena.allocatedBytes-arena.budgetBaselineBytes)
		if arena.budgetBytes > 0 && (used >= arena.budgetBytes || cost > arena.budgetBytes-used) {
			return false
		}
		arena.compactReuseDependencyIndex = make([]compactReuseDependencyIndexEntry, count)
		arena.allocatedBytes += cost
	} else {
		clear(arena.compactReuseDependencyIndex)
		arena.compactReuseDependencyIndex = arena.compactReuseDependencyIndex[:count]
	}
	i := 0
	arena.eachDenseDependency(func(node *Node, receipt incr.DenseReceipt) {
		arena.compactReuseDependencyIndex[i] = compactReuseDependencyIndexEntry{node: node, start: receipt.Start, end: uint64(receipt.End) + uint64(receipt.Lookahead)}
		i++
	})
	slices.SortFunc(arena.compactReuseDependencyIndex, func(a, b compactReuseDependencyIndexEntry) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})
	arena.buildCompactReuseDependencyIndexMax(0, count)
	arena.compactReuseDependencyIndexed = true
	return true
}

func (arena *nodeArena) buildCompactReuseDependencyIndexMax(lo, hi int) uint64 {
	if lo == hi {
		return 0
	}
	mid := lo + (hi-lo)/2
	entry := &arena.compactReuseDependencyIndex[mid]
	entry.maxEnd = max(entry.end, arena.buildCompactReuseDependencyIndexMax(lo, mid), arena.buildCompactReuseDependencyIndexMax(mid+1, hi))
	entry.live = true
	return entry.maxEnd
}

func (arena *nodeArena) invalidateIndexedCompactReuseDependencies(edit InputEdit, lo, hi int) (uint64, bool) {
	if lo == hi {
		return 0, false
	}
	index := arena.compactReuseDependencyIndex
	mid := lo + (hi-lo)/2
	entry := &index[mid]
	if !entry.live {
		return 0, false
	}
	tailShift := edit.NewEndByte != edit.OldEndByte || edit.NewEndPoint != edit.OldEndPoint
	if tailShift {
		if entry.maxEnd <= uint64(edit.StartByte) && index[hi-1].start < edit.OldEndByte {
			return entry.maxEnd, true
		}
	} else if entry.maxEnd <= uint64(edit.StartByte) || index[lo].start >= edit.OldEndByte {
		return entry.maxEnd, true
	}
	if entry.node != nil && compactReuseDependencyIntersectsEdit(entry.start, entry.end, edit) {
		arena.clearDenseDependency(entry.node)
		entry.node, entry.end = nil, 0
	}
	leftEnd, leftLive := arena.invalidateIndexedCompactReuseDependencies(edit, lo, mid)
	rightEnd, rightLive := arena.invalidateIndexedCompactReuseDependencies(edit, mid+1, hi)
	entry.maxEnd = max(entry.end, leftEnd, rightEnd)
	entry.live = entry.node != nil || leftLive || rightLive
	return entry.maxEnd, entry.live
}

func (tree *Tree) editCompactReuseDependencies(edit InputEdit) {
	tree.arena.editCompactReuseDependencies(edit)
	for _, arena := range tree.borrowedArena {
		if arena != tree.arena {
			arena.editCompactReuseDependencies(edit)
		}
	}
}

// Node.Edit has no Tree arena list. Visit public nodes through lazy entries
// without materializing their children. Pending entries use the containing arena.
func editCompactReuseDependenciesFromNode(root *Node, edit InputEdit) {
	if root == nil || inputEditIsNoop(edit) {
		return
	}
	seen := make(map[*nodeArena]struct{})
	type frame struct {
		arena *nodeArena
		entry stackEntry
	}
	stack := []frame{{root.ownerArena, newStackEntryNode(root.parseState, root)}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if node := stackEntryNode(current.entry); node != nil {
			if _, ok := seen[node.ownerArena]; !ok {
				node.ownerArena.editCompactReuseDependencies(edit)
				seen[node.ownerArena] = struct{}{}
			}
			for i := 0; i < nodeChildCountNoMaterialize(node); i++ {
				if child, ok := nodeChildEntryAtNoMaterialize(node, i); ok {
					stack = append(stack, frame{node.ownerArena, child})
				}
			}
			continue
		}
		if parent := stackEntryPendingParent(current.entry); parent != nil {
			for i := 0; i < parent.childEntryCount(); i++ {
				stack = append(stack, frame{current.arena, parent.childEntry(current.arena, i)})
			}
		}
	}
}
