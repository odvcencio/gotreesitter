package gotreesitter

import "bytes"

type reuseFrame struct {
	node       *Node
	underDirty bool
}

// reuseCursor incrementally walks reusable nodes from an old tree in
// pre-order, caching candidates for the current token start byte.
type reuseCursor struct {
	sourceLen uint32
	oldSource []byte
	newSource []byte
	// wholeSourceIdentical is computed once at reset. Dirty-candidate checks can
	// be numerous, so they must not rescan the complete buffer per candidate.
	wholeSourceIdentical bool
	minEditAt            uint32
	hasEdits             bool
	// edits is the old tree's recorded edit list (post-parse Tree.Edit calls),
	// in application order. It is needed to reverse-map a node's post-edit
	// (shifted) byte coordinates back to its pre-edit coordinates in oldSource
	// so the reuse byte-equality guard reads the correct old-source bytes. See
	// oldByteForNew / nodeBytesUnchanged.
	edits []InputEdit

	stack []reuseFrame
	next  *Node

	topLevelParent *Node
	topLevelIndex  int
	topLevelEnd    int
	// End byte of the first edited top-level child. CSS reparses this prefix
	// before considering sibling reuse because selector/declaration scanner
	// context is too sensitive for leaf reuse inside the edited rule.
	topLevelResumeByte uint32
	// topLevelSpliceLeading enables the LEADING-run block-splice (campaign
	// post-admission-frontier T2a): top-level items whose end byte is strictly
	// before minEditAt are byte-identical prefixes of the old tree, so they may
	// be spliced back as a block from the initial parser state before per-token
	// parsing reaches the edit -- the mirror of the trailing block-splice. When
	// true, topLevelIndex starts at 0 (the scan covers the leading run, the
	// edited item, then the trailing run in one monotonic forward walk); when
	// false, topLevelIndex starts after the edited item (trailing run only), the
	// pre-existing behavior. Kept false for the scanner-sensitive forest
	// languages (css, cmake), whose leading items keep their general-walk leaf
	// reuse -- see reset() and topLevelSiblingBlockSpliceEligible.
	topLevelSpliceLeading bool
	// disableLeadingSplice backs a test-only differential seam. It is set by the
	// parser before reset and is never enabled by production code.
	disableLeadingSplice bool

	cachedStart      uint32
	cachedStartValid bool
	cached           []*Node

	rejectDirty                   uint64
	rejectAncestorDirtyBeforeEdit uint64
	rejectHasError                uint64
	rejectInvalidSpan             uint64
	rejectOutOfBounds             uint64
	rejectRootNonLeafChanged      uint64
	// observedPreGotoStateMismatch counts top-level block-splice candidates whose
	// recorded ownership frontier differs from the live parser state. Leading
	// transfers and explicitly strict lanes reject the mismatch; established
	// trailing lanes retain their compatible-goto contract.
	observedPreGotoStateMismatch uint64
	rejectLargeNonLeaf           uint64
	rejectStaleNonLeafBoundary   uint64
	// rejectFragileNonLeaf counts interior-reuse candidates rejected by
	// Node.isFragile() -- see tryReuseSubtree's non-leaf fallback lane below
	// and the Parser.ReuseRejectFragileNonLeaf profile field (parser.go).
	rejectFragileNonLeaf uint64
	// rejectScannerUnquiescent counts reuse candidates rejected by the external
	// scanner checkpoint/quiescence gate (canReuseNodeWithExternalScannerCheck
	// point) -- either a checkpoint state mismatch or a refuted quiescence
	// proof (campaign O(edit) W4, external_scanner_quiescence.go). It stays 0
	// for certified stateless-scanner languages, whose every boundary is
	// proven quiescent. See the Parser.ReuseRejectScannerUnquiescent field.
	rejectScannerUnquiescent uint64
	// rejectFrontierProofUnavailable counts compact-materialized non-leaf
	// candidates that have exact scanner bytes but no proof that the recorded
	// reduction owned the live parser frontier. Leaves use their independent
	// shift and checkpoint checks; non-leaves stay barred until that proof exists.
	rejectFrontierProofUnavailable uint32
	forestFastPath                 bool
	// compactMaterialized marks old trees whose node states came from compact
	// replay. Checkpoint snapshots authenticate scanner state, but they do not
	// prove the parser frontier after a non-leaf reduction. Keep compact
	// checkpoint reuse at independently revalidated leaves until that ownership
	// proof exists.
	compactMaterialized        bool
	compactCheckpointedScanner bool
	languageName               string // cached for language-specific reuse safety policies
	strictTopLevelOwnership    bool   // forest trees and certified stateless scanners require the recorded normal-dispatch frontier
}

// reuseScratch holds reusable buffers for incremental reuse traversal.
type reuseScratch struct {
	stack []reuseFrame
	cache []*Node
}

func (c *reuseCursor) reset(oldTree *Tree, source []byte, scratch *reuseScratch) *reuseCursor {
	if oldTree == nil || oldTree.RootNode() == nil {
		return nil
	}
	if scratch == nil {
		scratch = &reuseScratch{}
	}

	c.sourceLen = uint32(len(source))
	c.oldSource = oldTree.source
	c.newSource = source
	c.wholeSourceIdentical = bytes.Equal(c.oldSource, c.newSource)
	c.minEditAt = 0
	c.hasEdits = len(oldTree.edits) > 0
	c.edits = oldTree.edits
	if c.hasEdits {
		c.minEditAt = oldTree.edits[0].StartByte
		for i := 1; i < len(oldTree.edits); i++ {
			if oldTree.edits[i].StartByte < c.minEditAt {
				c.minEditAt = oldTree.edits[i].StartByte
			}
		}
	}
	c.stack = scratch.stack[:0]
	c.next = nil
	c.cachedStart = 0
	c.cachedStartValid = false
	c.cached = scratch.cache[:0]
	c.rejectDirty = 0
	c.rejectAncestorDirtyBeforeEdit = 0
	c.rejectHasError = 0
	c.rejectInvalidSpan = 0
	c.rejectOutOfBounds = 0
	c.rejectRootNonLeafChanged = 0
	c.observedPreGotoStateMismatch = 0
	c.rejectLargeNonLeaf = 0
	c.rejectStaleNonLeafBoundary = 0
	c.rejectFragileNonLeaf = 0
	c.rejectScannerUnquiescent = 0
	c.rejectFrontierProofUnavailable = 0
	c.forestFastPath = oldTree.forestFastPath
	c.compactMaterialized = oldTree.compactMaterialized
	c.compactCheckpointedScanner = c.compactMaterialized && languageUsesExternalScannerCheckpoints(oldTree.language)
	c.languageName = ""
	// Every forest node records the GSS state that owned its original reduce.
	// A compatible goto alone cannot transfer that ownership after an edit, so
	// forest-built top-level candidates always require an exact pre-goto match.
	c.strictTopLevelOwnership = c.forestFastPath
	if oldTree.language != nil {
		c.languageName = oldTree.language.Name
		if stateless, ok := oldTree.language.ExternalScanner.(StatelessExternalScanner); ok {
			c.strictTopLevelOwnership = c.strictTopLevelOwnership || stateless.ExternalScannerIsStateless()
		}
	}

	root := oldTree.RootNode()
	c.stack = append(c.stack, reuseFrame{node: root})
	c.topLevelParent = nil
	c.topLevelIndex = 0
	c.topLevelEnd = 0
	c.topLevelResumeByte = 0
	c.topLevelSpliceLeading = false
	childCount := nodeChildCountNoMaterialize(root)
	if c.hasEdits && root != nil && childCount > 0 {
		firstAffected := -1
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(root, i)
			if !ok {
				continue
			}
			if stackEntryNodeEndByte(entry) > c.minEditAt {
				firstAffected = i
				break
			}
		}
		if firstAffected >= 0 {
			hasTrailing := firstAffected+1 < childCount
			hasLeading := firstAffected > 0
			// The leading-run splice (campaign post-admission-frontier T2a) is
			// barred for scanner-sensitive forest languages (css, cmake): their
			// leading top-level items keep the general-walk leaf reuse whose
			// selector/declaration scanner context rejectDirtyTopLevelPrefix
			// already protects. Extending the top-level scan over their leading
			// run would supersede that leaf reuse with a top-level scan that the
			// same guard rejects, forcing a full reparse of the clean prefix, so
			// leading splice stays off for them. See the CSS disposition note on
			// topLevelSiblingBlockSpliceEligible.
			leadingEligible := hasLeading &&
				!c.disableLeadingSplice &&
				!(c.forestFastPath && forestFastPathDirtyPrefixScannerSensitive(c.languageName))
			if hasTrailing || leadingEligible {
				c.topLevelParent = root
				c.topLevelEnd = childCount
				if entry, ok := nodeChildEntryAtNoMaterialize(root, firstAffected); ok {
					c.topLevelResumeByte = stackEntryNodeEndByte(entry)
				}
				if leadingEligible {
					// Cover the leading run: the monotonic top-level scan starts
					// at item 0, splices the byte-identical leading items, skips
					// the edited item (its bytes changed, so reusableIndexedEntry
					// rejects it), then resumes on the trailing run. The block-
					// splice loop (parser.go) consumes the leading items from the
					// initial parser state exactly as it consumes the trailing
					// run from the post-edit state.
					c.topLevelIndex = 0
					c.topLevelSpliceLeading = true
				} else {
					// Trailing run only (pre-existing behavior): the scan starts
					// at the first item after the edited one.
					c.topLevelIndex = firstAffected + 1
				}
			}
		}
	}
	return c
}

func (c *reuseCursor) commitScratch(scratch *reuseScratch) {
	if scratch == nil {
		return
	}
	scratch.stack = c.stack[:0]
	scratch.cache = c.cached[:0]
}

// releaseNodeRefs nils all *Node pointers so the GC can collect the arenas
// they reference. Call before returning a Parser to a pool to prevent arena
// retention via reuseCursor holding nodes from the last incremental parse.
// Backing arrays (stack, cached) are kept to avoid re-allocation next parse.
func (c *reuseCursor) releaseNodeRefs() {
	c.next = nil
	c.topLevelParent = nil
	c.oldSource = nil
	c.newSource = nil
	c.edits = nil
	c.forestFastPath = false
	c.compactMaterialized = false
	c.compactCheckpointedScanner = false
	c.languageName = ""
	if cap(c.stack) > 0 {
		clear(c.stack[:cap(c.stack)])
		c.stack = c.stack[:0]
	}
	if cap(c.cached) > 0 {
		clear(c.cached[:cap(c.cached)])
		c.cached = c.cached[:0]
	}
}

// releaseNodeRefs nils *Node pointers in the scratch buffers.
func (s *reuseScratch) releaseNodeRefs() {
	if cap(s.stack) > 0 {
		clear(s.stack[:cap(s.stack)])
		s.stack = s.stack[:0]
	}
	if cap(s.cache) > 0 {
		clear(s.cache[:cap(s.cache)])
		s.cache = s.cache[:0]
	}
}

func (c *reuseCursor) candidates(start uint32) []*Node {
	if c == nil {
		return nil
	}
	if c.cachedStartValid {
		if start == c.cachedStart {
			return c.cached
		}
		if start < c.cachedStart {
			return nil
		}
	}

	c.cached = c.cached[:0]
	c.cachedStart = start
	c.cachedStartValid = true
	if c.collectTopLevelCandidates(start) {
		return c.cached
	}

	for {
		n := c.peek()
		if n == nil {
			return c.cached
		}

		if n.startByte < start {
			c.pop()
			continue
		}
		if n.startByte > start {
			return c.cached
		}

		for {
			n = c.peek()
			if n == nil || n.startByte != start {
				return c.cached
			}
			c.cached = append(c.cached, c.pop())
		}
	}
}

// hasNonLeafCandidateAt reports whether any reuse candidate beginning exactly
// at start is a non-leaf subtree (ChildCount > 0). Campaign O(edit) W1b uses
// this to scope the pending-reduce settling (settleEagerDefaultReduceChainFor
// Reuse) to the ONLY situation it can help: a non-leaf sibling waiting to be
// spliced whose recorded PreGotoState is reachable only after the pending
// unconditional reduces settle. When no non-leaf candidate starts here,
// settling could only perturb the parse trajectory (for example, changing
// where a trailing extra/comment attaches) with no reuse to gain, so it is
// skipped. Leaf candidates never need settling -- a leaf shifts, it does not
// consult a goto state -- so they do not qualify.
func (c *reuseCursor) hasNonLeafCandidateAt(start uint32) bool {
	if c == nil {
		return false
	}
	for _, n := range c.candidates(start) {
		if n != nil && n.ChildCount() > 0 {
			return true
		}
	}
	return false
}

func (c *reuseCursor) collectTopLevelCandidates(start uint32) bool {
	if c == nil || c.topLevelParent == nil || c.topLevelIndex >= c.topLevelEnd {
		return false
	}
	for c.topLevelIndex < c.topLevelEnd {
		entry, ok := nodeChildEntryAtNoMaterialize(c.topLevelParent, c.topLevelIndex)
		if !ok || !stackEntryHasNode(entry) {
			c.topLevelIndex++
			continue
		}
		childStart := stackEntryNodeStartByte(entry)
		if childStart < start {
			c.topLevelIndex++
			continue
		}
		if childStart > start {
			return false
		}
		for c.topLevelIndex < c.topLevelEnd {
			entry, ok = nodeChildEntryAtNoMaterialize(c.topLevelParent, c.topLevelIndex)
			if !ok || !stackEntryHasNode(entry) {
				c.topLevelIndex++
				continue
			}
			if stackEntryNodeStartByte(entry) != start {
				return true
			}
			c.topLevelIndex++
			if !c.topLevelBlockCandidateBytes(stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry)) {
				// An edited-region item (the edited item, or a leading item whose
				// end byte touches the edit so its last token can shift): do not
				// offer it as a whole-item block candidate. Skipping it here leaves
				// its start position with no top-level candidate, so the parser
				// reparses it and the general walk supplies leaf-level reuse for
				// its unchanged interior -- the pre-existing per-item behavior.
				continue
			}
			if !c.reusableIndexedEntry(entry) {
				continue
			}
			n := nodeChildAtForReason(c.topLevelParent, c.topLevelIndex-1, materializeForEdit)
			if n != nil {
				if c.compactCheckpointedScanner && n.ChildCount() > 0 {
					// A clean compact sibling may carry exact scanner bytes while
					// its reduction frontier remains unproven. Leave it for the
					// parser and keep scanning later siblings on the next request.
					c.rejectFrontierProofUnavailable++
					continue
				}
				c.cached = append(c.cached, n)
			}
		}
		return true
	}
	return false
}

func (c *reuseCursor) reusableIndexedEntry(entry stackEntry) bool {
	if !stackEntryHasNode(entry) {
		return false
	}
	start := stackEntryNodeStartByte(entry)
	end := stackEntryNodeEndByte(entry)
	if c.rejectDirtyTopLevelPrefix(start) {
		c.rejectDirty++
		return false
	}
	if c.hasEdits && !c.nodeBytesUnchanged(start, end) {
		c.rejectDirty++
		return false
	}
	dirtyHere := stackEntryNodeDirty(entry)
	// A dirty entry was marked dirty by TRUE OVERLAP with an edit. Its post-edit
	// span may have been clamped to a surviving prefix/suffix, so only whole-
	// source equality can safely clear the bit (the edit+inverse undo case).
	if dirtyHere && c.sourceBytesIdentical() {
		setStackEntryDirty(entry, false)
		dirtyHere = false
	}
	if stackEntryNodeHasError(entry) {
		c.rejectHasError++
		return false
	}
	if end <= start {
		c.rejectInvalidSpan++
		return false
	}
	if end > c.sourceLen {
		c.rejectOutOfBounds++
		return false
	}
	if dirtyHere {
		c.rejectDirty++
		return false
	}
	return true
}

func (c *reuseCursor) peek() *Node {
	if c.next != nil {
		return c.next
	}
	c.next = c.advance()
	return c.next
}

func (c *reuseCursor) pop() *Node {
	n := c.peek()
	if n != nil && perfCountersEnabled {
		perfRecordReusePopped()
	}
	c.next = nil
	return n
}

func (c *reuseCursor) advance() *Node {
	for len(c.stack) > 0 {
		last := len(c.stack) - 1
		frame := c.stack[last]
		c.stack = c.stack[:last]
		cur := frame.node
		if cur == nil {
			continue
		}
		if perfCountersEnabled {
			perfRecordReuseVisited()
		}

		dirtyHere := cur.dirty()
		if dirtyHere {
			// A clamped dirty span can compare equal while omitting the edited
			// boundary byte. Whole-source equality is the proof that this is an
			// actual edit+inverse undo rather than a surviving slice.
			if c.sourceBytesIdentical() {
				cur.setDirty(false)
				dirtyHere = false
			}
		}

		childUnderDirty := frame.underDirty || dirtyHere

		childCount := nodeChildCountNoMaterialize(cur)
		if perfCountersEnabled {
			perfRecordReusePushed(childCount)
		}
		for i := childCount - 1; i >= 0; i-- {
			if entry, ok := nodeChildEntryAtNoMaterialize(cur, i); ok &&
				c.cachedStartValid &&
				stackEntryNodeEndByte(entry) <= c.cachedStart {
				continue
			}
			child := nodeChildAtForReason(cur, i, materializeForEdit)
			if child == nil {
				continue
			}
			c.stack = append(c.stack, reuseFrame{
				node:       child,
				underDirty: childUnderDirty,
			})
		}

		if frame.underDirty && c.hasEdits &&
			!c.nodeBytesUnchanged(cur.startByte, cur.endByte) {
			c.rejectAncestorDirtyBeforeEdit++
			continue
		}
		if cur.hasError() {
			c.rejectHasError++
			continue
		}
		if cur.endByte <= cur.startByte {
			c.rejectInvalidSpan++
			continue
		}
		if cur.endByte > c.sourceLen {
			c.rejectOutOfBounds++
			continue
		}
		if c.rejectDirtyTopLevelPrefix(cur.startByte) {
			c.rejectDirty++
			continue
		}
		if dirtyHere {
			c.rejectDirty++
			continue
		}
		return cur
	}
	return nil
}

// oldByteForNew reverse-maps a post-edit (shifted) byte position in the new
// source back to its position in the old (pre-edit) source, undoing the
// coordinate shifts Tree.Edit applied for each recorded edit. Edits are undone
// in reverse application order. It returns ok=false if p falls inside an edited
// region (its old position is not well-defined) — callers treat that as "cannot
// verify, do not reuse".
//
// For a single edit e (byteDelta = NewEndByte - OldEndByte):
//   - p <= e.StartByte : unchanged (position precedes the edit)
//   - p >= e.NewEndByte : in the shifted suffix; old = p - byteDelta
//   - otherwise         : inside the new edited region; not mappable
func (c *reuseCursor) oldByteForNew(p uint32) (uint32, bool) {
	cur := int64(p)
	for i := len(c.edits) - 1; i >= 0; i-- {
		e := c.edits[i]
		if cur <= int64(e.StartByte) {
			continue
		}
		if cur >= int64(e.NewEndByte) {
			cur -= int64(e.NewEndByte) - int64(e.OldEndByte)
			continue
		}
		return 0, false
	}
	if cur < 0 || cur > int64(len(c.oldSource)) {
		return 0, false
	}
	return uint32(cur), true
}

// rightBoundaryTouchedByEdit reports whether a post-edit byte boundary maps to
// the start of, or lies inside, any recorded edit. It walks edits backward so
// each comparison uses that edit's coordinate space. A subtree ending at such
// a boundary cannot prove that its final token still terminates there.
func (c *reuseCursor) rightBoundaryTouchedByEdit(end uint32) bool {
	cur := int64(end)
	for i := len(c.edits) - 1; i >= 0; i-- {
		e := c.edits[i]
		start := int64(e.StartByte)
		newEnd := int64(e.NewEndByte)
		if cur == start {
			return true
		}
		if cur < start {
			continue
		}
		if cur < newEnd {
			return true
		}
		cur -= newEnd - int64(e.OldEndByte)
	}
	return false
}

// nodeBytesUnchanged reports whether the node spanning [start,end) in the NEW
// source has byte-identical text to the same node in the OLD source. The node's
// coordinates are post-edit (Tree.Edit already shifted them), so the old-source
// slice must be taken at the reverse-mapped coordinates — comparing oldSource at
// the post-shift coordinates (the historical bug, issue #380) makes every
// length-changed suffix node spuriously "differ" and blocks all suffix reuse.
// A node that straddles or is contained in an edited region reverse-maps to a
// different-length old span (or fails to map), so bytes.Equal correctly reports
// it changed — the guard stays sound, it just now reads the right old bytes.
func (c *reuseCursor) nodeBytesUnchanged(start, end uint32) bool {
	if end < start {
		return false
	}
	if end > uint32(len(c.newSource)) {
		return false
	}
	oldStart, ok1 := c.oldByteForNew(start)
	oldEnd, ok2 := c.oldByteForNew(end)
	if !ok1 || !ok2 || oldEnd < oldStart {
		return false
	}
	return bytes.Equal(c.oldSource[oldStart:oldEnd], c.newSource[start:end])
}

// sourceBytesIdentical is the only sound basis for clearing a dirty bit after
// Tree.Edit. A dirty node's post-edit span may have been clamped to the edit
// boundary, so comparing that span alone can compare only a surviving prefix or
// suffix and falsely declare a truncated token unchanged. Whole-buffer equality
// retains the edit+inverse undo optimization without transferring ownership of
// a genuine edit back to incremental reuse.
func (c *reuseCursor) sourceBytesIdentical() bool {
	return c != nil && c.wholeSourceIdentical
}

func reuseSubtreeGapIsParserPadding(source []byte, stackByteOffset, nodeStart uint32, continuationEscape byte) bool {
	stack := glrStack{byteOffset: stackByteOffset}
	return realTokenAttachmentGapIsParserPadding(source, &stack, Token{StartByte: nodeStart}, nil, continuationEscape)
}

func reuseStackByteOffsetAfterTruncate(s *glrStack, depth int, entryScratch *glrEntryScratch) (uint32, bool) {
	if s == nil {
		return 0, false
	}
	if depth <= 0 {
		return 0, true
	}
	if depth >= s.depth() {
		return s.byteOffset, true
	}
	entries := s.ensureEntries(entryScratch)
	if depth > len(entries) {
		return 0, false
	}
	return stackByteOffset(entries[:depth]), true
}

// tryReuseSubtree attempts to reuse an old subtree at the current lookahead.
// On success it appends the reused node to the stack and returns the first
// lookahead token that begins at or after the node's end byte.
func (p *Parser) tryReuseSubtree(s *glrStack, lookahead Token, ts TokenSource, idx *reuseCursor, entryScratch *glrEntryScratch, gssScratch *gssScratch) (Token, uint32, bool) {
	continuationEscape := p.lineContinuationEscapeByte()
	candidates := idx.candidates(lookahead.StartByte)
	if perfCountersEnabled {
		perfRecordReuseCandidates(len(candidates))
	}
	if len(candidates) == 0 {
		return lookahead, 0, false
	}

	state := s.top().state
	for _, n := range candidates {
		if n.ChildCount() > 0 {
			if idx.compactCheckpointedScanner {
				idx.rejectFrontierProofUnavailable++
				continue
			}
			// Preserve full-root reuse on undo when bytes are identical.
			fullRootUndo := n.startByte == 0 &&
				n.endByte == idx.sourceLen &&
				idx.nodeBytesUnchanged(n.startByte, n.endByte)
			if !fullRootUndo && !idx.topLevelSiblingBlockSpliceEligible(n) {
				idx.rejectRootNonLeafChanged++
				continue
			}
			// A compatible goto target does not prove that this old reduction was
			// owned by the live normal-dispatch frontier. Explicitly strict lanes
			// and newly admitted leading blocks require the recorded pre-goto
			// state; established trailing-only lanes retain their measured
			// compatible-goto contract.
			if !fullRootUndo && !topLevelCandidateOwnsCurrentFrontier(n, state) {
				idx.observedPreGotoStateMismatch++
				if idx.strictTopLevelOwnership || idx.topLevelSpliceLeading {
					continue
				}
			}
		}
		nextState, ok := p.reuseTargetState(state, n, lookahead)
		if !ok {
			continue
		}
		if !reuseSubtreeGapIsParserPadding(idx.newSource, s.byteOffset, n.StartByte(), continuationEscape) {
			continue
		}
		cp, ok := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, state, n, lookahead.StartByte)
		if !ok {
			idx.rejectScannerUnquiescent++
			continue
		}
		return reuseNode(p, s, n, nextState, state, lookahead, ts, idx, entryScratch, gssScratch, cp)
	}

	// Conservative fallback: try non-root non-leaf nodes. This increases reuse
	// surface without jumping to reuse candidates that can trigger expensive
	// recovery behavior.
	//
	// maxNonLeafReuseSpan was originally a small (2048-byte) safety cap
	// standing in for a real soundness proof: C tree-sitter's
	// ts_parser__reuse_node never reuses a subtree whose reduce happened
	// under an LR-table conflict, a GSS multi-pop, or concurrent GLR stack
	// versions (its is_fragile check -- see markReduceFragility,
	// parser_reduce.go, and Node.isFragile, tree.go). This codebase had no
	// equivalent metadata, so span was the only cheap proxy for "probably
	// simple enough to be safe" available, and interior reuse for large
	// subtrees stayed off (see issue #380). Now that every reduce marks
	// fragility precisely, the isFragile() check below is the actual
	// soundness gate and span is just a sanity backstop against pathological
	// candidates, so it can be raised far past the old byte-count heuristic.
	const maxNonLeafReuseSpan = 1 << 20
	for _, n := range candidates {
		if n == nil || n.ChildCount() == 0 || n.parent == nil {
			continue
		}
		// Compact replay records exact scanner snapshots, but a checkpoint at a
		// reduction boundary does not identify the parser state for the next
		// token. A length-changing indentation edit can keep that snapshot equal
		// while changing whether the next statement belongs to the reduction.
		// Reuse leaves, whose shift state and token span are independently checked;
		// reparse compact non-leaf reductions until frontier ownership is recorded.
		if idx.compactCheckpointedScanner {
			idx.rejectFrontierProofUnavailable++
			continue
		}
		span := n.EndByte() - n.StartByte()
		if span == 0 || span > maxNonLeafReuseSpan {
			if span > maxNonLeafReuseSpan {
				idx.rejectLargeNonLeaf++
			}
			continue
		}
		// C-equivalent soundness gate (see the maxNonLeafReuseSpan comment
		// above): never splice a fragile subtree into a fresh parse. A
		// fragile n may look byte-identical to what a clean reparse would
		// produce, but its shape depended on which ambiguous derivation won
		// at parse time -- surrounding context this edit may have changed --
		// so reusing it here would silently corrode structural correctness.
		if n.isFragile() {
			idx.rejectFragileNonLeaf++
			continue
		}
		// A non-leaf ending exactly at an edit start cannot prove that its
		// final token still terminates there; keep that boundary in reparse.
		if idx.rightBoundaryTouchedByEdit(n.EndByte()) {
			idx.rejectStaleNonLeafBoundary++
			continue
		}
		// Without an exact scanner checkpoint, retain the conservative token
		// boundary proof: a fresh lookahead ending elsewhere means the old
		// subtree's maximal-munch boundary is stale. Checkpointed scanners use
		// the stronger proof below instead: exact live pre-goto ownership plus
		// an exact serialized scanner state at the candidate boundary.
		if !languageUsesExternalScannerCheckpoints(p.language) {
			if leaf := leftmostLeaf(n); leaf == nil || leaf.EndByte() != lookahead.EndByte {
				idx.rejectStaleNonLeafBoundary++
				continue
			}
		}
		nextState, truncateDepth, ok := p.reuseNonLeafTargetStateOnStack(s, n)
		if !ok {
			continue
		}
		reuseByteOffset := s.byteOffset
		if truncateDepth > 0 && truncateDepth < s.depth() {
			var ok bool
			reuseByteOffset, ok = reuseStackByteOffsetAfterTruncate(s, truncateDepth, entryScratch)
			if !ok {
				continue
			}
		}
		if !reuseSubtreeGapIsParserPadding(idx.newSource, reuseByteOffset, n.StartByte(), continuationEscape) {
			continue
		}
		if truncateDepth > 0 && truncateDepth < s.depth() {
			if !s.truncate(truncateDepth) {
				continue
			}
		}
		startState := s.top().state
		cp, ok := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, startState, n, lookahead.StartByte)
		if !ok {
			idx.rejectScannerUnquiescent++
			continue
		}
		return reuseNode(p, s, n, nextState, startState, lookahead, ts, idx, entryScratch, gssScratch, cp)
	}

	return lookahead, 0, false
}

func (c *reuseCursor) requiredTopLevelOwnershipFrontier(start uint32) (StateID, bool) {
	if c == nil || (!c.strictTopLevelOwnership && !c.topLevelSpliceLeading) {
		return 0, false
	}
	for _, n := range c.candidates(start) {
		if n != nil && n.ChildCount() > 0 && c.topLevelSiblingBlockSpliceEligible(n) {
			return n.PreGotoState(), true
		}
	}
	return 0, false
}

// topLevelCandidateOwnsCurrentFrontier is deliberately stricter than
// reuseTargetState: the latter answers whether a node can be shifted/goto'd
// from a state, while this answers whether a whole top-level item's original
// reduce ownership can be transferred there. In conflict-sensitive grammars
// those are different propositions.
func topLevelCandidateOwnsCurrentFrontier(n *Node, state StateID) bool {
	return n != nil && state == n.PreGotoState()
}

// topLevelSiblingBlockSpliceEligible reports whether n -- a candidate sitting
// directly under the old tree's top-level parent, at or after the first
// unaffected sibling's start byte -- may be spliced back as a whole block via
// the primary (cheap, current-state) reuseTargetState check instead of
// falling through to the conservative per-node fallback loop below. This is
// campaign O(edit) workstream W1 (spec.campaign.oedit): after reparsing the
// edited item, the run of following untouched top-level siblings is offered
// to reuseTargetState directly, driving ReuseRejectRootNonLeafChanged toward
// zero on every language, not just a curated forest-fast-path allowlist.
//
// Admission is per-node, not per-language (decision-0008's rider and the
// campaign's "no name-allowlists" invariant): the caller already proved n
// starts at a top-level boundary with byte-identical old/new text
// (collectTopLevelCandidates -> reusableIndexedEntry's nodeBytesUnchanged
// gate), so the only remaining soundness question is whether n's own shape
// depended on an ambiguous parse decision that surrounding context (which
// this edit may have changed) could invalidate -- exactly what isFragile()
// answers (see markReduceFragility, parser_reduce.go, and the doc comment on
// nodeFlagFragileLeft/Right, tree.go). A fragile candidate is barred here and
// must fall through to the conservative fallback (or a fresh reparse of that
// item), same as any other fragile non-leaf candidate.
//
// External-scanner quiescence is not re-checked here: canReuseNodeWithExternal
// ScannerCheckpoint (called by the caller immediately after this returns true)
// already verifies scanner-state compatibility generically for every language
// that records checkpoints, and languageSupportsIncrementalReuse already
// barred this whole reuse route for any external scanner that has not
// declared itself safe (IncrementalReuseExternalScanner.SupportsIncrementalReuse)
// -- see parser.go's tokenSourceSupportsIncrementalReuse gate, which runs
// before tryReuseSubtree is ever reached.
// The leading run (campaign post-admission-frontier T2a). A top-level item
// whose end byte is at or before minEditAt lies entirely before the first
// edit, so its bytes are unchanged by construction (every recorded edit starts
// at minEditAt or later). Such an item is a byte-identical prefix, admissible on
// the same terms as a trailing sibling: the caller still verifies byte equality
// (collectTopLevelCandidates -> reusableIndexedEntry's nodeBytesUnchanged gate),
// external-scanner quiescence (canReuseNodeWithExternalScannerCheckpoint), the
// zero-width / has-error / span gates, and the #393 reuse bar, and the fragility
// bit below still bars any item whose shape depended on an ambiguous decision.
// The block-splice loop replays the leading run from the parser's initial state,
// the mirror of replaying the trailing run from the post-edit state.
//
// CSS disposition: the scanner-sensitive forest languages (css, cmake) never set
// topLevelSpliceLeading (reset()), so their leading items are not admitted here
// and keep their general-walk leaf reuse. Their rejectDirtyTopLevelPrefix guard
// exists precisely because a whole-item splice of scanner-sensitive prefix
// content can feed stale selector/declaration context; leaving leading splice
// off for them respects that guard rather than relying on W4 quiescence to cover
// it.
func (c *reuseCursor) topLevelSiblingBlockSpliceEligible(n *Node) bool {
	if c == nil || c.topLevelParent == nil || n == nil || n.parent != c.topLevelParent || n.isFragile() {
		return false
	}
	return c.topLevelBlockCandidateBytes(n.startByte, n.endByte)
}

// topLevelBlockCandidateBytes reports whether a top-level item spanning
// [start,end) may be spliced back as a whole block by byte range alone (the
// fragility and scanner gates are applied separately by the caller). Two disjoint
// runs qualify:
//
//   - Trailing run: start >= topLevelResumeByte (the edited item's end byte).
//     These sit entirely after the edit; their byte-range equality is verified by
//     the caller. This is the pre-existing (trailing) admission, unchanged.
//   - Leading run: end < minEditAt, and only when the leading splice is enabled
//     for this tree (topLevelSpliceLeading). The STRICT `<` is load-bearing: it
//     requires the byte at index `end` -- the byte that terminated the item's
//     last token -- to lie before the first edit, so it is unedited and that
//     token boundary is preserved. An item ending exactly at minEditAt
//     (end == minEditAt) is excluded: an edit there can extend its last token by
//     maximal munch (for example replacing the newline after "package p" makes
//     "package pz"), which a whole-item splice would miss. Such a boundary item
//     is left to the general walk / reparse, exactly as before this change.
func (c *reuseCursor) topLevelBlockCandidateBytes(start, end uint32) bool {
	if start >= c.topLevelResumeByte {
		return true
	}
	return c.topLevelSpliceLeading && end < c.minEditAt
}

// forestFastPathDirtyPrefixScannerSensitive names the curated set of
// forest-fast-path languages (cmake, css) whose grammar-level scanner context
// around the first edited top-level item is sensitive enough that reusing
// leaves inside it can feed stale context (selector-vs-declaration state,
// etc.) and produce a fresh-tree mismatch. This is a REJECTION-only defensive
// guard scoped to trees built by the GSS-forest fast path
// (reuseCursor.forestFastPath, glr_forest.go) -- unrelated to and NOT
// admitted by topLevelSiblingBlockSpliceEligible's fragility gate above, so
// it does not fall under the campaign's "no name-allowlists" admission rule
// (that rule targets what reuse eligibility grants, not what it forbids).
// Extend only for another forest-fast-path language with the same proven
// scanner-context hazard, not as a general admission list.
func forestFastPathDirtyPrefixScannerSensitive(name string) bool {
	switch name {
	case "cmake", "css":
		return true
	default:
		return false
	}
}

// rejectDirtyTopLevelPrefix forces scanner-sensitive forest languages to reparse
// the first edited top-level item. Reusing equal leaves inside that dirty item
// can feed stale external-scanner context and produce a fresh-tree mismatch.
func (c *reuseCursor) rejectDirtyTopLevelPrefix(start uint32) bool {
	return c != nil &&
		c.forestFastPath &&
		forestFastPathDirtyPrefixScannerSensitive(c.languageName) &&
		c.topLevelParent != nil &&
		c.topLevelResumeByte > 0 &&
		start < c.topLevelResumeByte
}

// blockSpliceScannerSkipEligible reports whether a reused subtree may be
// spliced with an O(1) byte skip (SkipToByte, no token-by-token re-lex) on a
// non-checkpoint external-scanner language. Campaign O(edit) W1 block-splice
// (spec.campaign.oedit) needs this so a whole run of unchanged top-level
// siblings costs O(siblings), not O(bytes reused): without it every reused
// sibling re-lexes its own body (advanceTokenSourceTo) purely to keep the live
// external-scanner state exact for the token AFTER the span, re-lexing the file
// the reuse was meant to skip.
//
// Soundness. The gate is external-scanner QUIESCENCE at the span start: the
// scanner's Serialize captures zero bytes. By the tree-sitter scanner contract
// (Serialize must persist all state that changes any later scan()), an empty
// serialization proves the scanner holds no forward-affecting state, so the
// live scanner is indistinguishable from a fresh empty scanner for every future
// scan(). Skipping the span's bytes without lexing them therefore leaves the
// next real token byte-identical to what re-lexing would produce. For a
// stateless scanner -- one whose Serialize is unconditionally zero, e.g. Go's
// (grammars/go_scanner.go) -- this holds at the span start AND end
// unconditionally, so the skip is byte-exact by construction, never a
// heuristic. For a stateful scanner the residual obligation is that a complete,
// error-free subtree returns the scanner to an empty serialized state at its
// end; that is exactly the per-language external-scanner quiescence proof
// campaign workstream W4 formalizes, and it is enforced meanwhile by the
// full-serialization incremental invariant gate across every corpus language
// (python is excluded here: it takes the recorded-checkpoint path, not this
// one). Checkpoint languages never reach this helper.
func blockSpliceScannerSkipEligible(dts *dfaTokenSource) bool {
	return dts != nil && dts.externalScannerQuiescent()
}

func reuseNode(p *Parser, s *glrStack, n *Node, nextState StateID, startState StateID, lookahead Token, ts TokenSource, idx *reuseCursor, entryScratch *glrEntryScratch, gssScratch *gssScratch, checkpoint externalScannerCheckpointRef) (Token, uint32, bool) {
	if perfCountersEnabled {
		perfRecordReuseSuccess()
		if n.ChildCount() == 0 {
			perfRecordReuseLeafSuccess()
		} else {
			perfRecordReuseNonLeafSuccess(n.EndByte() - n.StartByte())
		}
	}
	p.pushStackNode(s, nextState, n, entryScratch, gssScratch)
	// A fresh parse accumulates every reduction's dynamic precedence into the
	// stack score; a reused subtree carries that same contribution precomputed
	// in its cumulative dynamicPrecedence. Credit it so score-sensitive
	// decisions (merge survival, culls, result selection) see the score a
	// fresh parse of the identical structure would.
	s.score += int(n.dynamicPrecedence)
	reusedBytes := n.EndByte() - n.StartByte()

	// If the reused node reaches EOF, we can synthesize EOF directly
	// instead of consuming every trailing token.
	if n.EndByte() == idx.sourceLen {
		pt := n.EndPoint()
		return Token{
			Symbol:     0,
			StartByte:  idx.sourceLen,
			EndByte:    idx.sourceLen,
			StartPoint: pt,
			EndPoint:   pt,
		}, reusedBytes, true
	}

	// dfaTokenSource fast skip does not preserve external-scanner state.
	// For checkpointed scanner languages, only reuse nodes when the start
	// parser/scanner state matches exactly, then restore the recorded end
	// snapshot before skipping to the node end.
	if dts := underlyingDFATokenSource(ts); dts != nil && dts.language != nil && dts.language.ExternalScanner != nil {
		if languageUsesExternalScannerCheckpoints(dts.language) {
			if stateful, ok := ts.(parserStateTokenSource); ok {
				stateful.SetParserState(nextState)
				stateful.SetGLRStates(nil)
			}
			if startState != n.PreGotoState() {
				return lookahead, 0, false
			}
			if checkpoint == (externalScannerCheckpointRef{}) {
				if skipper, ok := ts.(PointSkippableTokenSource); ok {
					return skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint()), reusedBytes, true
				}
				if skipper, ok := ts.(ByteSkippableTokenSource); ok {
					return skipper.SkipToByte(n.EndByte()), reusedBytes, true
				}
				return advanceTokenSourceTo(ts, lookahead, n.EndByte()), reusedBytes, true
			}
			if tok, ok := fastForwardWithExternalScannerCheckpoint(ts, n, checkpoint); ok {
				return tok, reusedBytes, true
			}
		}
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(nextState)
			stateful.SetGLRStates(nil)
		}
		// Campaign O(edit) W1 block-splice (spec.campaign.oedit): a
		// non-checkpoint external-scanner language normally re-lexes every
		// token inside the reused span (advanceTokenSourceTo) so the live
		// scanner state stays exact for the token AFTER the span. That makes a
		// whole-run sibling splice cost O(bytes reused), re-lexing the file the
		// reuse was meant to avoid. When the scanner is provably QUIESCENT --
		// its Serialize captures zero bytes right here -- it carries no state
		// that can affect any later scan() (the tree-sitter scanner contract:
		// Serialize must persist every bit of state that changes future
		// lexing). Skipping the intervening bytes without lexing therefore
		// leaves the next real token identical to what re-lexing would produce,
		// so the O(1) byte skip is byte-exact, not a heuristic. It is scoped by
		// blockSpliceScannerSkipEligible below to hold at BOTH the span start
		// (checked here) and the span end.
		if blockSpliceScannerSkipEligible(dts) {
			if skipper, ok := ts.(PointSkippableTokenSource); ok {
				return skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint()), reusedBytes, true
			}
			if skipper, ok := ts.(ByteSkippableTokenSource); ok {
				return skipper.SkipToByte(n.EndByte()), reusedBytes, true
			}
		}
		return advanceTokenSourceTo(ts, lookahead, n.EndByte()), reusedBytes, true
	}

	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(nextState)
			stateful.SetGLRStates(nil)
		}
		return skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint()), reusedBytes, true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(nextState)
			stateful.SetGLRStates(nil)
		}
		return skipper.SkipToByte(n.EndByte()), reusedBytes, true
	}

	return advanceTokenSourceTo(ts, lookahead, n.EndByte()), reusedBytes, true
}

func advanceTokenSourceTo(ts TokenSource, lookahead Token, endByte uint32) Token {
	tok := lookahead
	for tok.Symbol != 0 && tok.EndByte <= endByte {
		next := ts.Next()
		// Defensive break for non-advancing token sources.
		if next.StartByte == tok.StartByte && next.EndByte == tok.EndByte {
			return next
		}
		tok = next
	}
	return tok
}

func (p *Parser) reuseTargetState(state StateID, n *Node, lookahead Token) (StateID, bool) {
	// Leaf reuse must match the current lookahead token symbol.
	if n.ChildCount() == 0 {
		if n.Symbol() != lookahead.Symbol {
			return 0, false
		}
		// The caller always lexes the fresh lookahead at the candidate's start
		// byte before consulting reuse (tryReuseSubtree selects candidates by
		// n.startByte == lookahead.StartByte). If a fresh lex of the same
		// symbol at that position ends at a different byte than the stored
		// leaf, the leaf's token boundary is stale (a maximal-munch decision
		// that depended on bytes at or beyond the leaf's old right edge no
		// longer holds under the new source) and reusing it would truncate or
		// extend the real token. Reject rather than reuse.
		if n.EndByte() != lookahead.EndByte {
			return 0, false
		}

		action := p.lookupAction(state, n.Symbol())
		if action == nil || len(action.Actions) == 0 {
			return 0, false
		}
		var uniqueShiftState StateID
		shiftCount := 0
		for _, act := range action.Actions {
			if act.Type != ParseActionShift {
				continue
			}
			targetState := act.State
			// Extra-token shifts keep the parser state unchanged.
			if act.Extra {
				targetState = state
			}
			if targetState == n.parseState {
				return targetState, true
			}
			if shiftCount == 0 {
				uniqueShiftState = targetState
			}
			shiftCount++
		}
		if n.parseState == 0 && shiftCount == 1 {
			return uniqueShiftState, true
		}
		return 0, false
	}

	if perfCountersEnabled {
		perfRecordReuseNonLeafCheck()
	}
	gotoState := p.lookupGoto(state, n.Symbol())
	if gotoState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafNoGoto()
			if p.language != nil && int(n.Symbol()) < int(p.language.TokenCount) {
				perfRecordReuseNonLeafNoGotoTerminal()
			} else {
				perfRecordReuseNonLeafNoGotoNonTerminal()
			}
		}
		return 0, false
	}
	if n.parseState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateZero()
		}
	}
	return gotoState, true
}

// leftmostLeaf returns the leftmost leaf (childless) descendant of n. A
// node's span always starts where its first child's span starts, so this
// walks child index 0 down through the tree; the result shares n's
// StartByte. Returns nil if n is nil or a leftmost leaf cannot be reached
// (e.g. materialization fails).
func leftmostLeaf(n *Node) *Node {
	for n != nil && n.ChildCount() > 0 {
		child := nodeChildAtForReason(n, 0, materializeForEdit)
		if child == n {
			// Defensive: avoid an infinite loop on a malformed self-referential
			// child link.
			return nil
		}
		n = child
	}
	return n
}

func (p *Parser) reuseNonLeafTargetStateOnStack(s *glrStack, n *Node) (StateID, int, bool) {
	if s == nil || n == nil || n.ChildCount() == 0 {
		return 0, 0, false
	}
	if perfCountersEnabled {
		perfRecordReuseNonLeafCheck()
	}

	preGoto := n.PreGotoState()
	// The recorded pre-goto state identifies the parser frontier that owned
	// this reduction. Finding the same state deeper in the stack is not
	// sufficient: truncating back to it can discard newly reparsed syntax and
	// transfer ownership across a recovery or scanner boundary.
	if s.top().state != preGoto {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateMiss()
		}
		return 0, 0, false
	}

	gotoState := p.lookupGoto(preGoto, n.Symbol())
	if gotoState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafNoGoto()
			if p.language != nil && int(n.Symbol()) < int(p.language.TokenCount) {
				perfRecordReuseNonLeafNoGotoTerminal()
			} else {
				perfRecordReuseNonLeafNoGotoNonTerminal()
			}
		}
		return 0, 0, false
	}
	if n.parseState != 0 && gotoState != n.parseState {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateMiss()
		}
		return 0, 0, false
	}

	return gotoState, s.depth(), true
}
