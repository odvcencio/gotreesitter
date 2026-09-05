package gotreesitter

import (
	"slices"
	"unsafe"
)

type compactReuseDependency struct {
	start, end, lookaheadBytes uint32
	column                     uint32
}

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
func setCompactReuseDependency(node *Node, lookaheadBytes uint32) bool {
	if node == nil || node.ownerArena == nil {
		return false
	}
	arena := node.ownerArena
	arena.compactReuseDependencyMu.Lock()
	defer arena.compactReuseDependencyMu.Unlock()
	if prior, ok := arena.compactReuseDependencies[node]; ok {
		if prior.start == node.startByte && prior.end == node.endByte && prior.column == node.startPoint.Column {
			lookaheadBytes = maxUint32(lookaheadBytes, prior.lookaheadBytes)
		}
		arena.compactReuseDependencies[node] = compactReuseDependency{node.startByte, node.endByte, lookaheadBytes, node.startPoint.Column}
		arena.compactReuseDependencyIndexed = false
		return true
	}
	// Include map growth and deleted slots. Do not reclaim the charge on deletion.
	const entryBytes = int64(96)
	cost := entryBytes
	if arena.compactReuseDependencies == nil {
		cost += 256
	}
	used := max(int64(0), arena.allocatedBytes-arena.budgetBaselineBytes)
	if arena.budgetBytes > 0 && (used >= arena.budgetBytes || cost > arena.budgetBytes-used) {
		return false
	}
	if arena.compactReuseDependencies == nil {
		arena.compactReuseDependencies = make(map[*Node]compactReuseDependency)
	}
	arena.compactReuseDependencies[node] = compactReuseDependency{node.startByte, node.endByte, lookaheadBytes, node.startPoint.Column}
	arena.compactReuseDependencyIndexed = false
	arena.compactReuseDependencyEntries++
	arena.allocatedBytes += cost
	return true
}

func compactReuseDependencyForNode(node *Node) (uint32, bool) {
	if node == nil || node.ownerArena == nil {
		return 0, false
	}
	arena := node.ownerArena
	arena.compactReuseDependencyMu.RLock()
	receipt, ok := arena.compactReuseDependencies[node]
	arena.compactReuseDependencyMu.RUnlock()
	return receipt.lookaheadBytes, ok && receipt.start == node.startByte && receipt.end == node.endByte && receipt.column == node.startPoint.Column
}

func clearCompactReuseDependency(node *Node) {
	if node != nil && node.ownerArena != nil {
		arena := node.ownerArena
		arena.compactReuseDependencyMu.Lock()
		delete(arena.compactReuseDependencies, node)
		arena.compactReuseDependencyIndexed = false
		arena.compactReuseDependencyMu.Unlock()
	}
}

func (arena *nodeArena) compactReuseDependencyBytesAllocated() int64 {
	if arena == nil {
		return 0
	}
	bytes := int64(cap(arena.compactReuseDependencyIndex)) * int64(unsafe.Sizeof(compactReuseDependencyIndexEntry{}))
	if arena.compactReuseDependencies != nil {
		bytes += 256 + 96*int64(arena.compactReuseDependencyEntries)
	}
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
	if len(arena.compactReuseDependencies) == 0 {
		return
	}
	if arena.indexCompactReuseDependencies() {
		arena.invalidateIndexedCompactReuseDependencies(edit, 0, len(arena.compactReuseDependencyIndex))
		return
	}
	// Keep exact invalidation when the budget cannot fund an index.
	for node, receipt := range arena.compactReuseDependencies {
		end := uint64(receipt.end) + uint64(receipt.lookaheadBytes)
		if compactReuseDependencyIntersectsEdit(receipt.start, end, edit) {
			delete(arena.compactReuseDependencies, node)
		}
	}
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
	count := len(arena.compactReuseDependencies)
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
	for node, receipt := range arena.compactReuseDependencies {
		arena.compactReuseDependencyIndex[i] = compactReuseDependencyIndexEntry{
			node: node, start: receipt.start, end: uint64(receipt.end) + uint64(receipt.lookaheadBytes),
		}
		i++
	}
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
		delete(arena.compactReuseDependencies, entry.node)
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
