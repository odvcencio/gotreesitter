package gotreesitter

import (
	"bytes"
	"sort"
	"unsafe"
)

type externalScannerCheckpoint struct {
	start []byte
	end   []byte
}

type externalScannerSnapshotRef struct {
	off  uint32
	slab uint16
	len  uint16
}

type externalScannerCheckpointRef struct {
	start externalScannerSnapshotRef
	end   externalScannerSnapshotRef
}

func externalScannerCheckpointRefComplete(cp externalScannerCheckpointRef) bool {
	return cp.start.len != 0 && cp.end.len != 0
}

type externalScannerCheckpointSet struct {
	indexes []uint32
	refs    []externalScannerCheckpointRef
}

func languageUsesExternalScannerCheckpoints(lang *Language) bool {
	if lang == nil || lang.ExternalScanner == nil {
		return false
	}
	checkpointed, ok := lang.ExternalScanner.(CheckpointedExternalScanner)
	return ok && checkpointed.UsesExternalScannerCheckpoints()
}

func languageRequiresExternalScannerPrefixFrontierProof(lang *Language) bool {
	if lang == nil || lang.ExternalScanner == nil {
		return false
	}
	prefixSensitive, ok := lang.ExternalScanner.(IncrementalPrefixFrontierExternalScanner)
	return ok && prefixSensitive.RequiresIncrementalPrefixFrontierProof()
}

func languageAllowsCheckpointlessExternalReuse(lang *Language) bool {
	if lang == nil || lang.ExternalScanner == nil {
		return false
	}
	checkpointless, ok := lang.ExternalScanner.(CheckpointlessExternalScannerReuse)
	return ok && checkpointless.AllowsIncrementalReuseWithoutCheckpoint()
}

func underlyingDFATokenSource(ts TokenSource) *dfaTokenSource {
	switch src := ts.(type) {
	case *dfaTokenSource:
		return src
	case *includedRangeTokenSource:
		return underlyingDFATokenSource(src.base)
	default:
		return nil
	}
}

func (a *nodeArena) recordExternalScannerLeafCheckpoint(node *Node, start, end []byte) bool {
	if a == nil || node == nil || len(start) == 0 || len(end) == 0 {
		return false
	}
	startRef := a.copyExternalScannerSnapshotRef(start)
	endRef := startRef
	if !bytes.Equal(start, end) {
		endRef = a.copyExternalScannerSnapshotRef(end)
	}
	ok := a.setExternalScannerCheckpoint(node, externalScannerCheckpointRef{
		start: startRef,
		end:   endRef,
	})
	if ok {
		a.externalScannerCheckpointRecords++
	}
	return ok
}

func (a *nodeArena) recordExternalScannerCompactCheckpoint(start, end []byte) externalScannerCheckpointRef {
	if a == nil || len(start) == 0 || len(end) == 0 {
		return externalScannerCheckpointRef{}
	}
	startRef := a.copyExternalScannerSnapshotRef(start)
	endRef := startRef
	if !bytes.Equal(start, end) {
		endRef = a.copyExternalScannerSnapshotRef(end)
	}
	return externalScannerCheckpointRef{
		start: startRef,
		end:   endRef,
	}
}

func (a *nodeArena) copyExternalScannerSnapshotRef(src []byte) externalScannerSnapshotRef {
	if a == nil || len(src) == 0 {
		return externalScannerSnapshotRef{}
	}
	if bytes.Equal(src, a.externalScannerSnapshotBytes(a.externalScannerLastSnapshotRef)) {
		return a.externalScannerLastSnapshotRef
	}
	ref := a.allocExternalScannerSnapshotRef(src)
	a.externalScannerLastSnapshotRef = ref
	return ref
}

func (a *nodeArena) setExternalScannerCheckpoint(node *Node, cp externalScannerCheckpointRef) bool {
	if a == nil || node == nil {
		return false
	}
	set, idx, ok := a.externalScannerCheckpointSetForNode(node, true)
	if !ok {
		return false
	}
	a.allocatedBytes += set.upsert(idx, cp)
	return true
}

func externalScannerCheckpointForNode(node *Node) (externalScannerCheckpoint, bool) {
	cp, ok := externalScannerCheckpointRefForNode(node)
	if !ok || node == nil || node.ownerArena == nil {
		return externalScannerCheckpoint{}, false
	}
	return externalScannerCheckpoint{
		start: node.ownerArena.externalScannerSnapshotBytes(cp.start),
		end:   node.ownerArena.externalScannerSnapshotBytes(cp.end),
	}, true
}

func externalScannerCheckpointRefForNode(node *Node) (externalScannerCheckpointRef, bool) {
	if node == nil || node.ownerArena == nil {
		return externalScannerCheckpointRef{}, false
	}
	set, idx, ok := node.ownerArena.externalScannerCheckpointSetForNode(node, false)
	if !ok {
		return externalScannerCheckpointRef{}, false
	}
	cp, ok := set.lookup(idx)
	if !ok || !externalScannerCheckpointRefComplete(cp) ||
		!node.ownerArena.externalScannerSnapshotRefValid(cp.start) ||
		!node.ownerArena.externalScannerSnapshotRefValid(cp.end) {
		return externalScannerCheckpointRef{}, false
	}
	return cp, true
}

// copyExternalScannerCheckpointToNode preserves checkpoint ownership when
// parser shaping clones or relabels a node. Checkpoints are arena sidecars, so
// copying a Node header alone cannot carry them to the new arena slot.
func copyExternalScannerCheckpointToNode(dst, src *Node) bool {
	if dst == nil || src == nil || dst.ownerArena == nil || src.ownerArena == nil {
		return false
	}
	if !dst.ownerArena.inheritExternalScannerCheckpointIdentity(src.ownerArena) {
		return false
	}
	cp, ok := externalScannerCheckpointRefForNode(src)
	if !ok {
		return false
	}
	if dst.ownerArena != src.ownerArena {
		cp, ok = cloneExternalScannerCheckpointRef(src.ownerArena, dst.ownerArena, cp)
		if !ok {
			return false
		}
	}
	if !dst.ownerArena.setExternalScannerCheckpoint(dst, cp) {
		return false
	}
	dst.ownerArena.externalScannerCheckpointRecords++
	return true
}

// recordExternalScannerCheckpointForParent derives a reduction receipt from
// checkpoints already owned by its shaped children. It deliberately performs
// direct sidecar lookups rather than recursively rebuilding descendants: parse
// construction is bottom-up, so recursion here would make deep reductions
// quadratic.
func recordExternalScannerCheckpointForParent(parent *Node, children []*Node) bool {
	if parent == nil || parent.ownerArena == nil || len(children) == 0 {
		return false
	}
	var start externalScannerSnapshotRef
	var end externalScannerSnapshotRef
	startOK := false
	endOK := false
	for _, child := range children {
		cp, ok := externalScannerCheckpointRefForNode(child)
		if !ok {
			continue
		}
		start = copyExternalScannerSnapshotRefBetweenArenas(child.ownerArena, parent.ownerArena, cp.start)
		startOK = start.len != 0
		break
	}
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		cp, ok := externalScannerCheckpointRefForNode(child)
		if !ok {
			continue
		}
		end = copyExternalScannerSnapshotRefBetweenArenas(child.ownerArena, parent.ownerArena, cp.end)
		endOK = end.len != 0
		break
	}
	if !startOK || !endOK {
		return false
	}
	if !parent.ownerArena.setExternalScannerCheckpoint(parent, externalScannerCheckpointRef{start: start, end: end}) {
		return false
	}
	parent.ownerArena.externalScannerCheckpointRecords++
	return true
}

func copyExternalScannerSnapshotRefBetweenArenas(src, dst *nodeArena, ref externalScannerSnapshotRef) externalScannerSnapshotRef {
	if src == nil || dst == nil || ref.len == 0 {
		return externalScannerSnapshotRef{}
	}
	if src == dst {
		return ref
	}
	return dst.copyExternalScannerSnapshotRef(src.externalScannerSnapshotBytes(ref))
}

func rebuildExternalScannerCheckpoints(root *Node, lang *Language) {
	if root == nil || !languageUsesExternalScannerCheckpoints(lang) {
		return
	}
	rebuildExternalScannerCheckpointForNode(root)
}

func rebuildExternalScannerCheckpointForNode(n *Node) (externalScannerCheckpointRef, bool) {
	if n == nil || n.ownerArena == nil {
		return externalScannerCheckpointRef{}, false
	}
	if cp, ok := externalScannerCheckpointRefForNode(n); ok {
		return cp, true
	}
	childCount := nodeChildCountNoMaterialize(n)
	if childCount == 0 {
		return externalScannerCheckpointRef{}, false
	}
	var startBytes []byte
	var endBytes []byte
	startOK := false
	endOK := false
	for i := 0; i < childCount; i++ {
		cp, ok := externalScannerCheckpointForChild(n, i)
		if ok {
			startBytes = cp.start
			startOK = true
			break
		}
	}
	for i := childCount - 1; i >= 0; i-- {
		cp, ok := externalScannerCheckpointForChild(n, i)
		if ok {
			endBytes = cp.end
			endOK = true
			break
		}
	}
	if !startOK || !endOK {
		return externalScannerCheckpointRef{}, false
	}
	startRef := n.ownerArena.copyExternalScannerSnapshotRef(startBytes)
	endRef := startRef
	if !bytes.Equal(startBytes, endBytes) {
		endRef = n.ownerArena.copyExternalScannerSnapshotRef(endBytes)
	}
	cp := externalScannerCheckpointRef{start: startRef, end: endRef}
	if !externalScannerCheckpointRefComplete(cp) ||
		!n.ownerArena.externalScannerSnapshotRefValid(cp.start) ||
		!n.ownerArena.externalScannerSnapshotRefValid(cp.end) ||
		!n.ownerArena.setExternalScannerCheckpoint(n, cp) {
		return externalScannerCheckpointRef{}, false
	}
	n.ownerArena.externalScannerCheckpointRecords++
	return cp, true
}

func externalScannerCheckpointForChild(parent *Node, childIndex int) (externalScannerCheckpoint, bool) {
	if parent == nil || parent.ownerArena == nil {
		return externalScannerCheckpoint{}, false
	}
	entry, ok := nodeChildEntryAtNoMaterialize(parent, childIndex)
	if !ok {
		child := nodeChildAtForReason(parent, childIndex, materializeForCheckpointRebuild)
		if _, ok := rebuildExternalScannerCheckpointForNode(child); !ok {
			return externalScannerCheckpoint{}, false
		}
		return externalScannerCheckpointForNode(child)
	}
	return externalScannerCheckpointForStackEntry(parent.ownerArena, entry)
}

func externalScannerCheckpointForStackEntry(arena *nodeArena, entry stackEntry) (externalScannerCheckpoint, bool) {
	if !stackEntryHasNode(entry) {
		return externalScannerCheckpoint{}, false
	}
	if node := stackEntryNode(entry); node != nil {
		if _, ok := rebuildExternalScannerCheckpointForNode(node); !ok {
			return externalScannerCheckpoint{}, false
		}
		return externalScannerCheckpointForNode(node)
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		if !leaf.hasCheckpoint || arena == nil ||
			!arena.externalScannerSnapshotRefValid(leaf.checkpoint.start) ||
			!arena.externalScannerSnapshotRefValid(leaf.checkpoint.end) {
			return externalScannerCheckpoint{}, false
		}
		return externalScannerCheckpoint{
			start: arena.externalScannerSnapshotBytes(leaf.checkpoint.start),
			end:   arena.externalScannerSnapshotBytes(leaf.checkpoint.end),
		}, true
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		return externalScannerCheckpointForPendingParent(arena, parent)
	}
	return externalScannerCheckpoint{}, false
}

func externalScannerCheckpointForPendingParent(arena *nodeArena, parent *pendingParent) (externalScannerCheckpoint, bool) {
	if arena == nil || parent == nil {
		return externalScannerCheckpoint{}, false
	}
	childCount := parent.childEntryCount()
	if childCount == 0 {
		return externalScannerCheckpoint{}, false
	}
	var startBytes []byte
	var endBytes []byte
	startOK := false
	endOK := false
	for i := 0; i < childCount; i++ {
		cp, ok := externalScannerCheckpointForStackEntry(arena, parent.childEntry(arena, i))
		if ok {
			startBytes = cp.start
			startOK = true
			break
		}
	}
	for i := childCount - 1; i >= 0; i-- {
		cp, ok := externalScannerCheckpointForStackEntry(arena, parent.childEntry(arena, i))
		if ok {
			endBytes = cp.end
			endOK = true
			break
		}
	}
	if !startOK || !endOK {
		return externalScannerCheckpoint{}, false
	}
	return externalScannerCheckpoint{start: startBytes, end: endBytes}, true
}

func rebuildExternalScannerCheckpointForMaterializedParent(n *Node, reason materializeReason) {
	if n == nil || n.ownerArena == nil {
		return
	}
	switch reason {
	case materializeForEdit, materializeForCheckpointRebuild:
	default:
		return
	}
	if nodeChildCountNoMaterialize(n) == 0 {
		return
	}
	rebuildExternalScannerCheckpointForNode(n)
}

func currentExternalScannerCheckpoint(ts TokenSource) (externalScannerCheckpoint, uint32, uint32, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return externalScannerCheckpoint{}, 0, 0, false
	}
	return dts.lastExternalScannerCheckpoint()
}

func canReuseNodeWithExternalScannerCheckpoint(ts TokenSource, startState StateID, node *Node) (externalScannerCheckpointRef, bool) {
	return canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, startState, node, ^uint32(0))
}

func canReuseNodeWithExternalScannerCheckpointAtLookahead(ts TokenSource, startState StateID, node *Node, lookaheadStart uint32) (externalScannerCheckpointRef, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil {
		return externalScannerCheckpointRef{}, true
	}
	if !languageUsesExternalScannerCheckpoints(dts.language) {
		// Non-checkpoint external-scanner path. The W4 scanner-quiescence
		// classifier is the per-boundary authority here (campaign O(edit),
		// spec.campaign.oedit). A scanner satisfying the stateless proof in
		// external_scanner_quiescence.go is quiescent at every boundary, so
		// reuse is sound and returns a zero checkpoint the non-checkpoint reuse
		// path never reads. A scanner that opts out of
		// incremental reuse is refuted, and the caller fails closed. Every
		// other language keeps the legacy blanket admission, so this stays
		// neutral for production languages today.
		return externalScannerCheckpointRef{}, externalScannerBoundaryQuiescentWithoutCheckpoint(dts.language)
	}
	if node == nil || startState != node.PreGotoState() {
		return externalScannerCheckpointRef{}, false
	}
	// An identity-bearing scanner may not reuse a raw checkpoint from an arena
	// that lacks the same grammar and scanner identity. This check runs before
	// checkpointless admission, so equal raw bytes cannot bypass the proof.
	if !node.ownerArena.externalScannerCheckpointIdentityMatches(dts.language) {
		return externalScannerCheckpointRef{}, false
	}
	cp, ok := externalScannerCheckpointRefForNode(node)
	if !ok {
		return externalScannerCheckpointRef{}, languageAllowsCheckpointlessExternalReuse(dts.language)
	}
	want := node.ownerArena.externalScannerSnapshotBytes(cp.start)
	if !dts.externalScannerStateAtLookaheadStartMatches(want, lookaheadStart) {
		return externalScannerCheckpointRef{}, false
	}
	return cp, true
}

func fastForwardWithExternalScannerCheckpoint(ts TokenSource, node *Node, cp externalScannerCheckpointRef) (Token, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return Token{}, false
	}
	if node == nil || !externalScannerCheckpointRefComplete(cp) {
		return Token{}, false
	}
	dts.restoreExternalScannerState(node.ownerArena.externalScannerSnapshotBytes(cp.end))
	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		return skipper.SkipToByteWithPoint(node.EndByte(), node.EndPoint()), true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		return skipper.SkipToByte(node.EndByte()), true
	}
	return advanceTokenSourceTo(ts, Token{
		StartByte:  node.StartByte(),
		EndByte:    node.StartByte(),
		StartPoint: node.StartPoint(),
		EndPoint:   node.StartPoint(),
	}, node.EndByte()), true
}

func (a *nodeArena) externalScannerCheckpointSetForNode(node *Node, create bool) (*externalScannerCheckpointSet, int, bool) {
	if a == nil || node == nil {
		return nil, 0, false
	}
	if idx, ok := nodeIndexInStorage(node, a.nodes); ok {
		return &a.externalScannerNodeCheckpoints, idx, true
	}
	for i := range a.nodeSlabs {
		idx, ok := nodeIndexInStorage(node, a.nodeSlabs[i].data)
		if !ok {
			continue
		}
		if create {
			for len(a.externalScannerNodeCheckpointSlabs) <= i {
				a.externalScannerNodeCheckpointSlabs = append(a.externalScannerNodeCheckpointSlabs, externalScannerCheckpointSlab{})
			}
		} else if i >= len(a.externalScannerNodeCheckpointSlabs) {
			return nil, 0, false
		}
		return &a.externalScannerNodeCheckpointSlabs[i].checkpoints, idx, true
	}
	return nil, 0, false
}

func (s *externalScannerCheckpointSet) lookup(idx int) (externalScannerCheckpointRef, bool) {
	if s == nil || len(s.indexes) == 0 || idx < 0 {
		return externalScannerCheckpointRef{}, false
	}
	key := uint32(idx)
	pos := sort.Search(len(s.indexes), func(i int) bool {
		return s.indexes[i] >= key
	})
	if pos >= len(s.indexes) || s.indexes[pos] != key {
		return externalScannerCheckpointRef{}, false
	}
	return s.refs[pos], true
}

func (s *externalScannerCheckpointSet) upsert(idx int, cp externalScannerCheckpointRef) int64 {
	if s == nil || idx < 0 {
		return 0
	}
	key := uint32(idx)
	n := len(s.indexes)
	if n == 0 || s.indexes[n-1] < key {
		beforeIndexCap := cap(s.indexes)
		beforeRefCap := cap(s.refs)
		s.indexes = append(s.indexes, key)
		s.refs = append(s.refs, cp)
		return externalScannerCheckpointIndexBytesForCap(cap(s.indexes)-beforeIndexCap) +
			externalScannerCheckpointBytesForCap(cap(s.refs)-beforeRefCap)
	}
	before := s.bytesAllocated()
	pos := sort.Search(n, func(i int) bool {
		return s.indexes[i] >= key
	})
	if pos < n && s.indexes[pos] == key {
		s.refs[pos] = cp
		return 0
	}
	s.indexes = append(s.indexes, 0)
	copy(s.indexes[pos+1:], s.indexes[pos:])
	s.indexes[pos] = key
	s.refs = append(s.refs, externalScannerCheckpointRef{})
	copy(s.refs[pos+1:], s.refs[pos:])
	s.refs[pos] = cp
	return s.bytesAllocated() - before
}

func (s *externalScannerCheckpointSet) ensureCapacity(min int) int64 {
	if s == nil || min <= 0 || (cap(s.indexes) >= min && cap(s.refs) >= min) {
		return 0
	}
	before := s.bytesAllocated()
	if cap(s.indexes) < min {
		indexes := make([]uint32, len(s.indexes), min)
		copy(indexes, s.indexes)
		s.indexes = indexes
	}
	if cap(s.refs) < min {
		refs := make([]externalScannerCheckpointRef, len(s.refs), min)
		copy(refs, s.refs)
		s.refs = refs
	}
	return s.bytesAllocated() - before
}

func (s *externalScannerCheckpointSet) reset() {
	if s == nil {
		return
	}
	clear(s.refs)
	s.indexes = s.indexes[:0]
	s.refs = s.refs[:0]
}

func (s externalScannerCheckpointSet) bytesAllocated() int64 {
	return externalScannerCheckpointIndexBytesForCap(cap(s.indexes)) + externalScannerCheckpointBytesForCap(cap(s.refs))
}

func (s externalScannerCheckpointSet) slotsAllocated() uint64 {
	return uint64(cap(s.refs))
}

func nodeIndexInStorage(node *Node, storage []Node) (int, bool) {
	if node == nil || len(storage) == 0 {
		return 0, false
	}
	start := uintptr(unsafe.Pointer(&storage[0]))
	ptr := uintptr(unsafe.Pointer(node))
	size := unsafe.Sizeof(Node{})
	end := start + uintptr(len(storage))*size
	if ptr < start || ptr >= end {
		return 0, false
	}
	offset := ptr - start
	if offset%size != 0 {
		return 0, false
	}
	return int(offset / size), true
}
