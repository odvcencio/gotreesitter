package ccore

import (
	"bytes"
	"sync"
)

// Changed ranges (C/get_changed_ranges.c and C/tree.c, 0.27.1; the vendored copy is
// cgo_harness/internal/coracle/src).
//
// ChangedRanges is C ts_tree_get_changed_ranges (C/tree.c:114). It compares two
// trees using the same language tables and returns the byte ranges whose syntactic structure
// differs, in the coordinates of the new tree, in C's order.
//
// What must be true of the old tree (C's rule for ts_tree_get_changed_ranges):
// it was edited with every edit that the new source contains (Tree.Edit, C
// ts_tree_edit), so that its has_changes flags and positions describe the new
// source; and the new tree was parsed from the new source with the edited old
// tree as its old tree, or by any parse of that source. The function does not
// check this: with an old tree that lacks an edit it returns what C returns for
// the same two trees.
//
// Reads only: it takes no lock and changes neither tree. Both trees must be
// held (not released) and not edited or released by another goroutine during the
// call (the reader contract of Tree.Reader).
//
// Allocation: the iterator stacks and the range lists are scratch kept in a
// pool and reused between calls; the result is a fresh slice the caller owns.
// The call is not part of a parse request (it reads finished trees), so it is
// not charged to a ledger.

// Range is C's TSRange.
type Range struct {
	StartByte, EndByte uint32
	StartRow, StartCol uint32
	EndRow, EndCol     uint32
}

// errorStateID is C ERROR_STATE (C/error_costs.h:4).
const errorStateID = 0

// lengthMax is C LENGTH_MAX (C/length.h).
var lengthMax = Length{bytes: ^uint32(0), row: ^uint32(0), col: ^uint32(0)}

// lenMin is C length_min (C/length.h:21).
func lenMin(a, b Length) Length {
	if a.bytes < b.bytes {
		return a
	}
	return b
}

// rangeArray is C TSRangeArray.
type rangeArray []Range

// add is ts_range_array_add (C/get_changed_ranges.c:10): it merges a range that
// starts at or before the end of the last one, and drops an empty range.
func (a *rangeArray) add(start, end Length) {
	if n := len(*a); n > 0 {
		last := &(*a)[n-1]
		if start.bytes <= last.EndByte {
			last.EndByte = end.bytes
			last.EndRow, last.EndCol = end.row, end.col
			return
		}
	}
	if start.bytes < end.bytes {
		*a = append(*a, Range{StartByte: start.bytes, EndByte: end.bytes, StartRow: start.row, StartCol: start.col, EndRow: end.row, EndCol: end.col})
	}
}

// intersects is ts_range_array_intersects (C/get_changed_ranges.c:30).
func (a rangeArray) intersects(startIndex int, startByte, endByte uint32) bool {
	for i := startIndex; i < len(a); i++ {
		r := &a[i]
		if r.EndByte > startByte {
			if r.StartByte >= endByte {
				break
			}
			return true
		}
	}
	return false
}

// rangeArrayChanged is ts_range_array_get_changed_ranges (C/get_changed_ranges.c:46):
// the stretches that are in one list of included ranges and not in the other.
func rangeArrayChanged(oldR, newR []Range, diffs *rangeArray) {
	newIndex, oldIndex := 0, 0
	current := Length{}
	inOld, inNew := false, false
	for oldIndex < len(oldR) || newIndex < len(newR) {
		var nextOld, nextNew Length
		switch {
		case inOld:
			r := &oldR[oldIndex]
			nextOld = Length{bytes: r.EndByte, row: r.EndRow, col: r.EndCol}
		case oldIndex < len(oldR):
			r := &oldR[oldIndex]
			nextOld = Length{bytes: r.StartByte, row: r.StartRow, col: r.StartCol}
		default:
			nextOld = lengthMax
		}
		switch {
		case inNew:
			r := &newR[newIndex]
			nextNew = Length{bytes: r.EndByte, row: r.EndRow, col: r.EndCol}
		case newIndex < len(newR):
			r := &newR[newIndex]
			nextNew = Length{bytes: r.StartByte, row: r.StartRow, col: r.StartCol}
		default:
			nextNew = lengthMax
		}

		if nextOld.bytes < nextNew.bytes {
			if inOld != inNew {
				diffs.add(current, nextOld)
			}
			if inOld {
				oldIndex++
			}
			current = nextOld
			inOld = !inOld
		} else if nextNew.bytes < nextOld.bytes {
			if inOld != inNew {
				diffs.add(current, nextNew)
			}
			if inNew {
				newIndex++
			}
			current = nextNew
			inNew = !inNew
		} else {
			if inOld != inNew {
				diffs.add(current, nextNew)
			}
			if inOld {
				oldIndex++
			}
			if inNew {
				newIndex++
			}
			inOld = !inOld
			inNew = !inNew
			current = nextNew
		}
	}
}

// rangeArrayChangedEffective maps no explicitly configured ranges to C's
// default included range, which covers the full uint32 coordinate space.
func rangeArrayChangedEffective(oldR, newR []Range, diffs *rangeArray) {
	full := [1]Range{{EndByte: ^uint32(0), EndRow: ^uint32(0), EndCol: ^uint32(0)}}
	if len(oldR) == 0 {
		oldR = full[:]
	}
	if len(newR) == 0 {
		newR = full[:]
	}
	rangeArrayChanged(oldR, newR, diffs)
}

// crEntry is C TreeCursorEntry (C/tree_cursor.h) with a handle for the subtree.
type crEntry struct {
	h               Handle
	position        Length
	childIndex      uint32
	structuralIndex uint32
}

// crIter is C Iterator (C/get_changed_ranges.c:140); the last field is the 0.27.1 addition.
type crIter struct {
	s               *Store
	tab             *Tables
	stack           []crEntry
	visibleDepth    uint32
	inPadding       bool
	prevExternalTok Handle
}

// aliasAt is ts_language_alias_at (C/language.h:233) for a parent record.
func (it *crIter) aliasAt(parent Handle, structural uint32) Symbol {
	prod := it.s.ProductionID(parent)
	if prod == 0 || int(prod) >= len(it.tab.Alias) {
		return 0
	}
	seq := it.tab.Alias[prod]
	if int(structural) >= len(seq) {
		return 0
	}
	return seq[structural]
}

// reset is iterator_new (C/get_changed_ranges.c:148). The stack slice is reused.
func (it *crIter) reset(s *Store, tab *Tables, root Handle) {
	it.s, it.tab = s, tab
	it.stack = append(it.stack[:0], crEntry{h: root})
	it.visibleDepth = 1
	it.inPadding = false
	it.prevExternalTok = 0
}

// done is iterator_done (C/get_changed_ranges.c:169).
func (it *crIter) done() bool { return len(it.stack) == 0 }

func (it *crIter) back() *crEntry { return &it.stack[len(it.stack)-1] }

// startPosition is iterator_start_position (C/get_changed_ranges.c:173).
func (it *crIter) startPosition() Length {
	e := *it.back()
	if it.inPadding {
		return e.position
	}
	return lenAdd(e.position, it.s.padding(e.h))
}

// endPosition is iterator_end_position (C/get_changed_ranges.c:182).
func (it *crIter) endPosition() Length {
	e := *it.back()
	r := lenAdd(e.position, it.s.padding(e.h))
	if it.inPadding {
		return r
	}
	return lenAdd(r, it.s.size(e.h))
}

// treeIsVisible is iterator_tree_is_visible (C/get_changed_ranges.c:192).
func (it *crIter) treeIsVisible() bool {
	e := *it.back()
	if it.s.visible(e.h) {
		return true
	}
	if len(it.stack) > 1 {
		parent := it.stack[len(it.stack)-2].h
		return it.aliasAt(parent, e.structuralIndex) != 0
	}
	return false
}

// visibleState is iterator_get_visible_state (C/get_changed_ranges.c:206). It
// returns tree 0 when it finds none. alias carries across the loop as in C.
func (it *crIter) visibleState() (tree Handle, alias Symbol, start uint32) {
	i := len(it.stack) - 1
	if it.inPadding {
		if i == 0 {
			return
		}
		i--
	}
	for ; i >= 0; i-- {
		e := it.stack[i]
		if i > 0 {
			alias = it.aliasAt(it.stack[i-1].h, e.structuralIndex)
		}
		if it.s.visible(e.h) || alias != 0 {
			return e.h, alias, e.position.bytes
		}
	}
	return
}

// ascend is iterator_ascend (C/get_changed_ranges.c:239).
func (it *crIter) ascend() {
	if it.done() {
		return
	}
	if it.treeIsVisible() && !it.inPadding {
		it.visibleDepth--
	}
	if it.back().childIndex > 0 {
		it.inPadding = false
	}
	it.stack = it.stack[:len(it.stack)-1]
}

// lastExternal is ts_subtree_last_external_token (C/subtree.c:801).
func lastExternal(s *Store, tree Handle) Handle {
	if !s.hasExternalTokens(tree) {
		return 0
	}
	for s.childCount(tree) > 0 {
		kids := s.children(tree)
		next := Handle(0)
		for i := len(kids) - 1; i >= 0; i-- {
			if s.hasExternalTokens(kids[i]) {
				next = kids[i]
				break
			}
		}
		if next == 0 {
			break
		}
		tree = next
	}
	return tree
}

// descend is iterator_descend (C/get_changed_ranges.c:246).
func (it *crIter) descend(goal uint32) bool {
	if it.inPadding {
		return false
	}
	for {
		didDescend := false
		e := *it.back()
		position := e.position
		structural := uint32(0)
		kids := it.s.Children(e.h)
		for i := uint32(0); i < uint32(len(kids)); i++ {
			child := kids[i]
			childLeft := lenAdd(position, it.s.padding(child))
			childRight := lenAdd(childLeft, it.s.size(child))
			if childRight.bytes > goal {
				it.stack = append(it.stack, crEntry{h: child, position: position, childIndex: i, structuralIndex: structural})
				if it.treeIsVisible() {
					if childLeft.bytes > goal {
						it.inPadding = true
					} else {
						it.visibleDepth++
					}
					return true
				}
				didDescend = true
				break
			}
			position = childRight
			if !it.s.extra(child) {
				structural++
			}
			if last := lastExternal(it.s, child); last != 0 {
				it.prevExternalTok = last
			}
		}
		if !didDescend {
			return false
		}
	}
}

// advance is iterator_advance (C/get_changed_ranges.c:293).
func (it *crIter) advance() {
	if it.inPadding {
		it.inPadding = false
		if it.treeIsVisible() {
			it.visibleDepth++
		} else {
			it.descend(0)
		}
		return
	}
	for {
		if it.treeIsVisible() {
			it.visibleDepth--
		}
		e := it.stack[len(it.stack)-1]
		it.stack = it.stack[:len(it.stack)-1]
		if it.done() {
			return
		}
		parent := it.back().h
		childIndex := e.childIndex + 1
		if last := lastExternal(it.s, e.h); last != 0 {
			it.prevExternalTok = last
		}
		if it.s.childCount(parent) > childIndex {
			position := lenAdd(e.position, it.s.totalSize(e.h))
			structural := e.structuralIndex
			if !it.s.extra(e.h) {
				structural++
			}
			next := it.s.Children(parent)[childIndex]
			it.stack = append(it.stack, crEntry{h: next, position: position, childIndex: childIndex, structuralIndex: structural})
			if it.treeIsVisible() {
				if it.s.padding(next).bytes > 0 {
					it.inPadding = true
				} else {
					it.visibleDepth++
				}
			} else {
				it.descend(0)
			}
			break
		}
	}
}

type crComparison int

const (
	iterDiffers crComparison = iota
	iterMayDiffer
	iterMatches
)

// extStateEqual is ts_subtree_external_scanner_state_eq (C/subtree.c:1088): a
// handle with no scanner state has the empty state.
func extStateEqual(a, b *Store, ha, hb Handle) bool {
	return bytes.Equal(a.ExtState(ha), b.ExtState(hb))
}

// crCompare is iterator_compare (C/get_changed_ranges.c:348).
func crCompare(o, n *crIter) crComparison {
	oldTree, oldAlias, oldStart := o.visibleState()
	newTree, newAlias, newStart := n.visibleState()

	if oldTree == 0 && newTree == 0 {
		return iterMatches
	}
	if oldTree == 0 || newTree == 0 {
		return iterDiffers
	}
	oldSym, newSym := o.s.symbol(oldTree), n.s.symbol(newTree)
	if oldAlias != newAlias || oldSym != newSym {
		return iterDiffers
	}

	oldSize, newSize := o.s.size(oldTree).bytes, n.s.size(newTree).bytes
	oldState, newState := o.s.parseState(oldTree), n.s.parseState(newTree)
	oldExt, newExt := o.s.hasExternalTokens(oldTree), n.s.hasExternalTokens(newTree)
	oldCost, newCost := o.s.errorCost(oldTree), n.s.errorCost(newTree)

	if oldStart != newStart ||
		oldSym == symError ||
		oldSize != newSize ||
		oldState == stateNone ||
		newState == stateNone ||
		(oldState == errorStateID) != (newState == errorStateID) ||
		oldCost != newCost ||
		oldExt != newExt ||
		o.s.hasChanges(oldTree) ||
		(oldExt && !extStateEqual(o.s, n.s, o.prevExternalTok, n.prevExternalTok)) {
		return iterMayDiffer
	}
	return iterMatches
}

// crScratch is the reusable state of one call: two iterators and two range lists.
type crScratch struct {
	oldIt, newIt crIter
	diffs        rangeArray // included_range_differences
	results      rangeArray
}

var crPool sync.Pool

// Keep normal calls reusable, but do not let a single unusually deep or
// fragmented tree retain arbitrarily large buffers in the pool. Every retained
// slice is bounded independently; its elements contain no pointers.
const crMaxRetainedEntries = 4096

func (sc *crScratch) resetForPool() {
	oldStack, newStack := sc.oldIt.stack, sc.newIt.stack
	if cap(oldStack) > crMaxRetainedEntries {
		oldStack = nil
	} else {
		oldStack = oldStack[:0]
	}
	if cap(newStack) > crMaxRetainedEntries {
		newStack = nil
	} else {
		newStack = newStack[:0]
	}
	sc.oldIt = crIter{stack: oldStack}
	sc.newIt = crIter{stack: newStack}
	if cap(sc.diffs) > crMaxRetainedEntries {
		sc.diffs = nil
	} else {
		sc.diffs = sc.diffs[:0]
	}
	if cap(sc.results) > crMaxRetainedEntries {
		sc.results = nil
	} else {
		sc.results = sc.results[:0]
	}
}

// subtreeChangedRanges is ts_subtree_get_changed_ranges (C/get_changed_ranges.c:413).
func subtreeChangedRanges(sc *crScratch, oldS, newS *Store, tab *Tables, oldRoot, newRoot Handle) {
	results := &sc.results
	*results = (*results)[:0]
	oldIt, newIt := &sc.oldIt, &sc.newIt
	oldIt.reset(oldS, tab, oldRoot)
	newIt.reset(newS, tab, newRoot)

	diffIndex := 0

	position := oldIt.startPosition()
	nextPosition := newIt.startPosition()
	if position.bytes < nextPosition.bytes {
		results.add(position, nextPosition)
		position = nextPosition
	} else if position.bytes > nextPosition.bytes {
		results.add(nextPosition, position)
		nextPosition = position
	}

	for {
		comparison := crCompare(oldIt, newIt)

		// Included ranges that were excluded and are now included, or the reverse
		// (C/get_changed_ranges.c:449).
		if comparison == iterMatches && sc.diffs.intersects(diffIndex, position.bytes, oldIt.endPosition().bytes) {
			comparison = iterMayDiffer
		}

		isChanged := false
		switch comparison {
		case iterMatches:
			nextPosition = oldIt.endPosition()
		case iterMayDiffer:
			if oldIt.descend(position.bytes) {
				if !newIt.descend(position.bytes) {
					isChanged = true
					nextPosition = oldIt.endPosition()
				}
			} else if newIt.descend(position.bytes) {
				isChanged = true
				nextPosition = newIt.endPosition()
			} else {
				nextPosition = lenMin(oldIt.endPosition(), newIt.endPosition())
			}
		case iterDiffers:
			isChanged = true
			nextPosition = lenMin(oldIt.endPosition(), newIt.endPosition())
		}

		// Both iterators caught up to the current position (C/get_changed_ranges.c:499).
		for !oldIt.done() && oldIt.endPosition().bytes <= nextPosition.bytes {
			oldIt.advance()
		}
		for !newIt.done() && newIt.endPosition().bytes <= nextPosition.bytes {
			newIt.advance()
		}

		// Both at the same depth (C/get_changed_ranges.c:509).
		for oldIt.visibleDepth > newIt.visibleDepth {
			oldIt.ascend()
		}
		for newIt.visibleDepth > oldIt.visibleDepth {
			newIt.ascend()
		}

		if isChanged {
			results.add(position, nextPosition)
		}
		position = nextPosition

		// Position in the included range differences (C/get_changed_ranges.c:531).
		for diffIndex < len(sc.diffs) {
			if sc.diffs[diffIndex].EndByte <= position.bytes {
				diffIndex++
			} else {
				break
			}
		}
		if oldIt.done() || newIt.done() {
			break
		}
	}

	oldSize, newSize := oldS.totalSize(oldRoot), newS.totalSize(newRoot)
	if oldSize.bytes < newSize.bytes {
		results.add(oldSize, newSize)
	} else if newSize.bytes < oldSize.bytes {
		results.add(newSize, oldSize)
	}
}

// ChangedRanges is ts_tree_get_changed_ranges (C/tree.c:114). oldIncluded and
// newIncluded are the included ranges of the two trees (C TSTree.included_ranges;
// the old list as edited by ts_range_edit). The caller must supply ordered,
// non-overlapping ranges with coordinates matching the corresponding trees.
// Both trees must use tab, which supplies the alias sequences. These caller
// preconditions are not checked.
//
// ok is false, and nothing is computed, when a tree is nil or released.
// Invisible roots are unsupported and return ok=false. The result is a new
// slice the caller may keep; it is empty when nothing changed.
func ChangedRanges(oldT, newT *Tree, tab *Tables, oldIncluded, newIncluded []Range) (out []Range, ok bool) {
	if tab == nil || oldT == nil || newT == nil || oldT.Released() || newT.Released() {
		return nil, false
	}
	oldRoot, newRoot := oldT.Root(), newT.Root()
	if oldRoot == 0 || newRoot == 0 {
		return nil, false
	}
	// The cursor depth algorithm assumes each root is visible. A hidden root
	// can retain a nonzero depth after its iterator is exhausted and otherwise
	// make the depth-alignment loop run forever.
	if !oldT.Reader().visible(oldRoot) || !newT.Reader().visible(newRoot) {
		return nil, false
	}

	sc, _ := crPool.Get().(*crScratch)
	if sc == nil {
		sc = new(crScratch)
	}
	sc.diffs = sc.diffs[:0]
	rangeArrayChangedEffective(oldIncluded, newIncluded, &sc.diffs)
	subtreeChangedRanges(sc, oldT.Reader(), newT.Reader(), tab, oldRoot, newRoot)
	out = make([]Range, len(sc.results))
	copy(out, sc.results)
	// Drop store/table references and oversized buffers before pooling.
	sc.resetForPool()
	crPool.Put(sc)
	return out, true
}
