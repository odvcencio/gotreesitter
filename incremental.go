package gotreesitter

import (
	"bytes"
	"time"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

type reuseFrame struct {
	node       *Node
	underDirty bool
}

// reuseCursor incrementally walks reusable nodes from an old tree in
// pre-order, caching candidates for the current token start byte.
type reuseCursor struct {
	cEquivalentReuse      bool
	sharedFrontierReuse   bool
	unprovenStateMismatch bool
	unprovenReuse         bool
	sourceLen             uint32
	oldSource             []byte
	newSource             []byte
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
	compactRecovery                bool
	compactCheckpointedScanner     bool
	languageName                   string // cached for language-specific reuse safety policies
	oldRuntimeNoPolicyPruning      bool
	oldRecoveryCertified           bool
	strictTopLevelOwnership        bool // forest trees and certified stateless scanners require the recorded normal-dispatch frontier
	spanChangingEdit               bool
	// topLevelLeadingStartLimit bounds the leading splice run: a leading item
	// is admitted only when it starts before this byte. See reset. It sits
	// after the last bool so it uses the struct's trailing padding and keeps
	// the embedded Parser layout unchanged.
	topLevelLeadingStartLimit uint32
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

	c.unprovenStateMismatch = false
	c.unprovenReuse = false
	c.sourceLen = uint32(len(source))
	c.oldSource = oldTree.source
	c.newSource = source
	c.wholeSourceIdentical = incrementalEditsRestoreNodeSpans(oldTree.edits) && bytes.Equal(c.oldSource, c.newSource)
	c.minEditAt = 0
	c.hasEdits = len(oldTree.edits) > 0
	c.edits = oldTree.edits
	c.spanChangingEdit = false
	for _, edit := range c.edits {
		c.spanChangingEdit = c.spanChangingEdit || edit.OldEndByte != edit.NewEndByte || edit.OldEndPoint != edit.NewEndPoint
	}
	// Shifted nodes retain their read bounds. The live first token and the
	// parser-padding gap authenticate the boundary omitted by the projection.
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
	compactMaterialized := oldTree.compactMaterialized
	// These projections do not yet preserve every native reuse attribute.
	// Keep their established frontier proof until they do.
	certifiedForest := oldTree.arena != nil && oldTree.arena.legacyReuseReads.CertifiedForestAttributes()
	oldRuntime := oldTree.rawParseRuntime()
	c.oldRuntimeNoPolicyPruning = oldRuntime.noPolicyPruning
	c.oldRecoveryCertified = !oldRuntime.CRecoveryDroppedErrorForClean && incrementalRecoveryShapeCertified(oldTree.root)
	c.cEquivalentReuse = c.cEquivalentReuse && (!c.forestFastPath || certifiedForest) && (!compactMaterialized || compactNodeStateProofAvailable(oldTree.root)) &&
		oldRuntime.StopReason == ParseStopAccepted && !oldRuntime.Truncated && !oldRuntime.TokenSourceEOFEarly

	c.compactRecovery = compactMaterialized && oldTree.root != nil && oldTree.root.hasError()
	c.compactCheckpointedScanner = compactMaterialized && languageUsesExternalScannerCheckpoints(oldTree.language)
	c.languageName = ""
	// Every forest node records the GSS state that owned its original reduce.
	// A compatible goto alone cannot transfer that ownership after an edit, so
	// forest-built top-level candidates always require an exact pre-goto match.
	c.strictTopLevelOwnership = c.forestFastPath || c.compactRecovery || (c.cEquivalentReuse && c.spanChangingEdit)
	if oldTree.language != nil {
		c.languageName = oldTree.language.Name
		if stateless, ok := oldTree.language.ExternalScanner.(StatelessExternalScanner); ok {
			c.strictTopLevelOwnership = c.strictTopLevelOwnership || stateless.ExternalScannerIsStateless()
		}
	}

	root := oldTree.RootNode()
	_, completeReads := legacyReuseLookahead(root)
	c.sharedFrontierReuse = false
	c.stack = append(c.stack, reuseFrame{node: root})
	c.topLevelParent = nil
	c.topLevelIndex = 0
	c.topLevelEnd = 0
	c.topLevelResumeByte = 0
	c.topLevelSpliceLeading = false
	c.topLevelLeadingStartLimit = ^uint32(0)
	childCount := nodeChildCountNoMaterialize(root)
	if c.hasEdits && root != nil && childCount > 0 {
		firstAffected := -1
		if c.compactRecovery {
			// Tree.Edit can clamp a dirty child that lies in a deleted recovery
			// region to zero width. Prefer that direct dirty child before the
			// historical end-byte probe, or the first clean suffix sibling becomes
			// the apparent edited item and loses its reuse boundary.
			for i := 0; i < childCount; i++ {
				entry, ok := nodeChildEntryAtNoMaterialize(root, i)
				if !ok {
					continue
				}
				if stackEntryNodeDirty(entry) && stackEntryNodeStartByte(entry) <= c.minEditAt {
					firstAffected = i
					break
				}
			}
		}
		if firstAffected < 0 {
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
		}
		if firstAffected >= 0 {
			if c.cEquivalentReuse && c.spanChangingEdit {
				changed := nodeChildAtForReason(root, firstAffected, materializeForEdit)
				if first := leftmostLeaf(changed); first != nil && first.dirty() && first.EndByte() <= c.minEditAt {
					// A boundary edit can extend an entry token into a different
					// production. Its old read bound does not certify the wrapper's
					// reconstructed ownership across that moving boundary.
					c.unprovenReuse = true
				}
			}
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
					// An item's extent is decided by the token that follows
					// it: the parser reduces the item when it sees that token.
					// When the edit starts at or before the edited item's
					// first byte, the edit can change that token, so the
					// item just before the edited one is not admitted. For
					// example, typing "i" after "func g(a int) " in Go makes
					// "i" g's result type (issue #454). Every earlier leading
					// item is followed by an item that ends before the edit.
					if entry, ok := nodeChildEntryAtNoMaterialize(root, firstAffected); ok && stackEntryNodeStartByte(entry) >= c.minEditAt {
						if prev, ok := nodeChildEntryAtNoMaterialize(root, firstAffected-1); ok {
							c.topLevelLeadingStartLimit = stackEntryNodeStartByte(prev)
						}
					}
				} else {
					// Trailing run only (pre-existing behavior): the scan starts
					// at the first item after the edited one.
					c.topLevelIndex = firstAffected + 1
				}
			}
		}
	}
	// Do not walk every token through the group adapter when the old tree has
	// no independent top-level production with complete read dependencies.
	if c.cEquivalentReuse && !compactMaterialized && completeReads && c.topLevelParent != nil {
		for i := 0; i < childCount; i++ {
			n := nodeChildAtForReason(root, i, materializeForEdit)
			// Parent hints are deferred after an incremental result. Authenticate
			// direct ownership from this root's child slot, not an older hint.
			setNodeParentLink(n, root, i)
			if n != nil && n.ChildCount() > 0 && c.topLevelSiblingBlockSpliceEligible(n) {
				if _, known := legacyReuseLookahead(n); known {
					c.sharedFrontierReuse = true
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

// certifiesIncrementalResult requires complete dependencies for every reused
// node. Reused nodes are clean and nonfragile, so their recovery cost is zero;
// reuseNode preserves their dynamic precedence and each live stack's cost.
// New recovery must run under the fresh merge policy before it can qualify.
func (c *reuseCursor) certifiesIncrementalResult(tree *Tree, p *Parser, source []byte, mergeOverride int) bool {
	if c == nil || !c.cEquivalentReuse || !c.oldRecoveryCertified || c.unprovenReuse || c.unprovenStateMismatch || tree == nil || tree.root == nil {
		return false
	}
	rt := tree.rawParseRuntime()
	if rt.StopReason != ParseStopAccepted || rt.Truncated || rt.TokenSourceEOFEarly || rt.CRecoveryDroppedErrorForClean {
		return false
	}
	if tree.root.HasError() {
		if !incrementalRecoveryShapeCertified(tree.root) {
			return false
		}
		baseCap := p.resolveParseMergePerKeyCap(source, nil, 0)
		if p.resolveParseMergePerKeyCap(source, c, mergeOverride) != baseCap {
			return false
		}
		if rt.MaxStacksSeen >= fullParseInitialMaxStacks(p.language, p.maxConflictWidth, source) &&
			(!rt.noPolicyPruning || !c.oldRuntimeNoPolicyPruning) {
			return false
		}
	}
	return true
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
	c.compactRecovery = false
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
	// C rejects an error-bearing parent and then descends into its children.
	// The indexed top-level path has no child fallback, so disable it for a
	// recovery root and let advance perform that descent.
	if c.compactRecovery && c.topLevelParent.hasError() {
		// Keep the parent so direct clean siblings can still pass the ordinary
		// ownership and frontier checks after the indexed scan is disabled.
		c.topLevelIndex = c.topLevelEnd
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
				if n.ChildCount() > 0 && n.isFragile() {
					// The whole block will be rejected. Preserve the general
					// cursor's descent so its clean leaves remain candidates.
					return len(c.cached) != 0
				}
				if c.compactCheckpointedScanner && n.ChildCount() > 0 &&
					!(n.isCompactMaterialized() && compactNodeStateProofAvailable(n)) {
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
	if n := stackEntryNode(entry); n != nil && n.isCompactMaterialized() {
		if compactNodeRecoveryBearing(n) {
			return false
		}
		if !compactNodeStateProofAvailable(n) {
			return false
		}
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
		if cur.isCompactMaterialized() {
			if compactNodeRecoveryBearing(cur) {
				continue
			}
			if !compactNodeStateProofAvailable(cur) {
				continue
			}
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

// incrementalEditsRestoreNodeSpans reports whether edits that cancel out also
// restore every node span. Tree.Edit collapses the nodes that start or end
// inside a removed or replaced range, and a later insertion does not restore
// them. Two kinds of removal are safe:
//   - A same-width replacement of one byte. No node boundary can fall inside
//     a one-byte range, so the edit moves no span.
//   - A removal that exactly cancels the latest insertion that is still open,
//     as when a user types a character and deletes it again.
func incrementalEditsRestoreNodeSpans(edits []InputEdit) bool {
	var openBuf [8]InputEdit
	open := openBuf[:0]
	for _, edit := range edits {
		if edit.OldEndByte == edit.StartByte {
			if edit.NewEndByte != edit.StartByte {
				open = append(open, edit)
			}
			continue
		}
		if edit.OldEndByte == edit.StartByte+1 && edit.NewEndByte == edit.OldEndByte &&
			edit.NewEndPoint == edit.OldEndPoint {
			continue
		}
		last := len(open) - 1
		if last < 0 {
			return false
		}
		insertion := open[last]
		if edit.StartByte != insertion.StartByte || edit.OldEndByte != insertion.NewEndByte ||
			edit.NewEndByte != edit.StartByte || edit.StartPoint != insertion.StartPoint ||
			edit.OldEndPoint != insertion.NewEndPoint || edit.NewEndPoint != edit.StartPoint {
			return false
		}
		open = open[:last]
	}
	return true
}

// sourceBytesIdentical is the only sound basis for clearing a dirty bit after
// Tree.Edit. A dirty node's post-edit span may have been clamped to the edit
// boundary, so comparing that span alone can compare only a surviving prefix or
// suffix and falsely declare a truncated token unchanged. Whole-buffer equality
// retains the edit+inverse undo optimization without transferring ownership of
// a genuine edit back to incremental reuse.
//
// Whole-buffer equality is not enough after an edit that removes bytes. See
// incrementalEditsRestoreNodeSpans.
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
	if idx.cEquivalentReuse && (s.cPaused || (s.cRec != nil && s.top().state == cErrorState)) {
		return lookahead, 0, false
	}
	continuationEscape := p.lineContinuationEscapeByte()
	candidates := idx.candidates(lookahead.StartByte)
	if perfCountersEnabled {
		perfRecordReuseCandidates(len(candidates))
	}
	if len(candidates) == 0 {
		return lookahead, 0, false
	}

	state := s.top().state
	// Composite checkpoints are reconstructed from materialized descendants and
	// can miss scanner transitions hidden from the tree. A scanner can opt out
	// of composite reuse until it can prove those boundary snapshots complete;
	// its direct leaf checkpoints still support exact token-level restoration.
	dts := underlyingDFATokenSource(ts)
	scannerNeedsLeafReuse := dts != nil && languageUsesExternalScannerCheckpoints(dts.language) &&
		!languageSupportsCheckpointedNonLeafReuse(dts.language)
	for _, n := range candidates {
		if idx.cEquivalentReuse && s.cEverErrored && (!idx.topLevelSiblingBlockSpliceEligible(n) || (idx.spanChangingEdit && n.PreGotoState() != state)) {
			continue
		}
		structuralReuse := false
		if idx.cEquivalentReuse {
			_, structuralReuse = legacyReuseLookahead(n)
		}
		stateMismatch := false
		if n != nil && n.isCompactMaterialized() && !compactNodeMayBeReused(n) {
			continue
		}
		if n.ChildCount() > 0 {
			if scannerNeedsLeafReuse {
				continue
			}
			if idx.compactCheckpointedScanner &&
				!(n.isCompactMaterialized() && compactNodeStateProofAvailable(n)) {
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
			// compatible-goto contract. Compact-materialized candidates share
			// that contract: replay records the same pre-goto and parse states
			// production records for the same span, compactNodeMayBeReused has
			// already excluded recovery-bearing and unproven nodes above, and
			// error-bearing compact trees are strict through
			// strictTopLevelOwnership. Rejecting every compact candidate here
			// left compact old trees with leaf-only reuse (issue #454).
			if !fullRootUndo && !topLevelCandidateOwnsCurrentFrontier(n, state) {
				idx.observedPreGotoStateMismatch++
				stateMismatch = true
				if (idx.spanChangingEdit || !structuralReuse) && (idx.strictTopLevelOwnership || idx.topLevelSpliceLeading) {
					continue
				}
			}
		}
		if structuralReuse && !p.legacyCanReuseFreshLeaf(state, n, lookahead, ts) && (n.isFragile() || !p.legacyCanReuseFirstLeaf(state, n) || !legacyReuseMatchesLookahead(n, lookahead)) {
			continue
		}
		nextState, ok := p.reuseTargetState(state, n, lookahead)
		if !ok {
			continue
		}
		if !reuseSubtreeGapIsParserPadding(idx.newSource, s.byteOffset, n.StartByte(), continuationEscape) {
			continue
		}
		if !tokenSourceCanResumeAt(ts, n.EndByte()) {
			continue
		}
		cp, ok := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, state, n, lookahead.StartByte)
		if !ok {
			idx.rejectScannerUnquiescent++
			continue
		}
		if stateMismatch && !structuralReuse {
			idx.unprovenStateMismatch = true
		}
		if next, width, reused := reuseNode(p, s, n, nextState, state, lookahead, ts, idx, entryScratch, gssScratch, cp); reused {
			return next, width, true
		}
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
		if n == nil || n.ChildCount() == 0 || n.parent == nil || scannerNeedsLeafReuse {
			continue
		}
		if n.isCompactMaterialized() && !compactNodeMayBeReused(n) {
			continue
		}
		// Compact replay records exact scanner snapshots, but a checkpoint at a
		// reduction boundary does not identify the parser state for the next
		// token. A length-changing indentation edit can keep that snapshot equal
		// while changing whether the next statement belongs to the reduction.
		// Reuse leaves, whose shift state and token span are independently checked;
		// reparse compact non-leaf reductions until frontier ownership is recorded.
		if idx.compactCheckpointedScanner &&
			!(n.isCompactMaterialized() && compactNodeStateProofAvailable(n)) {
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
		if !tokenSourceCanResumeAt(ts, n.EndByte()) {
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
			if idx.cEquivalentReuse {
				// Reconstructing an ancestor frontier is not a normal-dispatch
				// certificate, even when the resulting goto is compatible.
				idx.unprovenStateMismatch = true
			}
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
//     The item must also start before topLevelLeadingStartLimit, which
//     excludes the item just before the edited one when the edit can change
//     the token that ended it (see reset).
func (c *reuseCursor) topLevelBlockCandidateBytes(start, end uint32) bool {
	if start >= c.topLevelResumeByte {
		return true
	}
	return c.topLevelSpliceLeading && end < c.minEditAt && start < c.topLevelLeadingStartLimit
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
	freshLeaf := idx.cEquivalentReuse && p.legacyCanReuseFreshLeaf(startState, n, lookahead, ts)
	if idx.cEquivalentReuse {
		_, known := legacyReuseLookahead(n)
		if s.cPaused || (s.cRec != nil && startState == cErrorState) ||
			(s.cEverErrored && (!idx.topLevelSiblingBlockSpliceEligible(n) || (idx.spanChangingEdit && n.PreGotoState() != startState))) || n.IsError() || n.HasError() || n.IsMissing() || (!freshLeaf && (!known || n.isFragile() || !p.legacyCanReuseFirstLeaf(startState, n) || !legacyReuseMatchesLookahead(n, lookahead))) {
			return lookahead, 0, false
		}
		if idx.spanChangingEdit && n.ChildCount() > 0 && n.PreGotoState() != startState {
			idx.unprovenStateMismatch = true
		}
	}
	if perfCountersEnabled {
		perfRecordReuseSuccess()
		if n.ChildCount() == 0 {
			perfRecordReuseLeafSuccess()
		} else {
			perfRecordReuseNonLeafSuccess(n.EndByte() - n.StartByte())
		}
	}
	if d := underlyingDFATokenSource(ts); d != nil && d.lexer.reuseReads != nil {
		if freshLeaf && lookahead.lexerLookaheadEndByte >= n.EndByte() {
			d.lexer.reuseReads.Record(int(n.StartByte()), lookahead.lexerLookaheadEndByte)
		} else if count, ok := legacyReuseLookahead(n); ok && uint64(n.EndByte())+uint64(count) <= uint64(^uint32(0)) {
			d.lexer.reuseReads.Record(int(n.StartByte()), n.EndByte()+count)
		} else {
			d.lexer.reuseReads.Abstain()
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

	// A checkpointed scanner can emit zero-width tokens after the last byte, such
	// as an implicit closing token. Restore its checkpoint and scan that
	// boundary rather than replacing those transitions with synthetic EOF.
	if n.EndByte() == idx.sourceLen && !languageUsesExternalScannerCheckpoints(p.language) {
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
			lexState := reuseFollowingTokenLexState(p.language, n, nextState)
			stateful.SetParserState(lexState)
			stateful.SetGLRStates(nil)
			tok := skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint())
			stateful.SetParserState(nextState)
			return tok, reusedBytes, true
		}
		return skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint()), reusedBytes, true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		if stateful, ok := ts.(parserStateTokenSource); ok {
			lexState := reuseFollowingTokenLexState(p.language, n, nextState)
			stateful.SetParserState(lexState)
			stateful.SetGLRStates(nil)
			tok := skipper.SkipToByte(n.EndByte())
			stateful.SetParserState(nextState)
			return tok, reusedBytes, true
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

// tryReuseSharedFrontier adapts the native certificates to the version group.
// Every head must reach one destination so the following lexer/scanner call
// is identical for all versions. Each version keeps its existing ancestry.
func (p *Parser) tryReuseSharedFrontier(stacks []glrStack, tok Token, ts TokenSource, idx *reuseCursor, scratch *parserScratch, arena *nodeArena, reuseState *parseReuseState, timing *incrementalParseTiming) (Token, uint32, bool) {
	if idx == nil || !idx.sharedFrontierReuse || len(stacks) < 2 || tok.Symbol == 0 {
		return tok, 0, false
	}
	dts := underlyingDFATokenSource(ts)
	if dts != nil && languageUsesExternalScannerCheckpoints(dts.language) &&
		!languageSupportsCheckpointedNonLeafReuse(dts.language) {
		return tok, 0, false
	}
	started := time.Time{}
	if timing != nil {
		started = time.Now()
	}
	var destination StateID
	var hasDestination bool
	var checkpoint externalScannerCheckpointRef
	node, ok := incr.SharedCandidate(idx.candidates(tok.StartByte), stacks,
		func(n *Node) bool {
			hasDestination = false
			if n == nil || n.ChildCount() == 0 || n.EndByte() <= n.StartByte() || !idx.topLevelSiblingBlockSpliceEligible(n) || !legacyReuseMatchesLookahead(n, tok) || !tokenSourceCanResumeAt(ts, n.EndByte()) {
				return false
			}
			_, known := legacyReuseLookahead(n)
			return known
		},
		func(s *glrStack, n *Node) bool {
			if s.dead || s.accepted || s.shifted || s.cPaused || (s.cRec != nil && s.top().state == cErrorState) ||
				(s.cEverErrored && idx.spanChangingEdit && n.PreGotoState() != s.top().state) || !p.legacyCanReuseFirstLeaf(s.top().state, n) || !reuseSubtreeGapIsParserPadding(idx.newSource, s.byteOffset, n.StartByte(), p.lineContinuationEscapeByte()) {
				return false
			}
			next, valid := p.reuseTargetState(s.top().state, n, tok)
			if !valid || (hasDestination && next != destination) {
				return false
			}
			cp, valid := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, s.top().state, n, tok.StartByte)
			if !valid {
				return false
			}
			destination, hasDestination, checkpoint = next, true, cp
			return true
		})
	if !ok {
		if timing != nil {
			timing.reuseNanos += time.Since(started).Nanoseconds()
		}
		return tok, 0, false
	}
	next, width, reused := reuseNode(p, &stacks[0], node, destination, stacks[0].top().state, tok, ts, idx, &scratch.entries, &scratch.gss, checkpoint)
	if !reused {
		return tok, 0, false
	}
	for i := 1; i < len(stacks); i++ {
		p.pushStackNode(&stacks[i], destination, node, &scratch.entries, &scratch.gss)
		stacks[i].score += int(node.dynamicPrecedence)
	}
	reuseState.markReused(node, arena)
	if timing != nil {
		timing.reuseNanos += time.Since(started).Nanoseconds()
		timing.reusedSubtrees++
		timing.reusedBytes += uint64(width)
	}
	return next, width, true
}

// leftmostLeaf returns the leftmost leaf (childless) descendant of n. A
// node's span always starts where its first child's span starts, so this
// walks child index 0 down through the tree; the result shares n's
// StartByte. Returns nil if n is nil or a leftmost leaf cannot be reached
// (e.g. materialization fails).
// reuseFollowingTokenLexState uses the state recorded on n's rightmost leaf only
// when the post-goto state selects the default DFA start state. This preserves
// pre-reduction lexing where the post-goto mode has no specialization, while
// retaining established reuse behavior for specialized modes.
func reuseFollowingTokenLexState(lang *Language, n *Node, postGoto StateID) StateID {
	if lang == nil || int(postGoto) >= len(lang.LexModes) ||
		lang.LexModes[postGoto].LexStateIndex() != 0 {
		return postGoto
	}
	if n == nil || n.ChildCount() == 0 {
		return postGoto
	}
	leaf := n
	for leaf != nil && leaf.ChildCount() > 0 {
		child := nodeChildAtForReason(leaf, leaf.ChildCount()-1, materializeForEdit)
		if child == leaf {
			return postGoto
		}
		leaf = child
	}
	if leaf == nil || leaf.parseState == 0 || int(leaf.parseState) >= len(lang.LexModes) {
		return postGoto
	}
	return leaf.parseState
}

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

const (
	legacyReuseKeyword   = uint32(1) << 31
	legacyReuseLeafKnown = uint32(1) << 30
	legacyReuseCountMask = legacyReuseLeafKnown - 1
)

// The legacy facade keeps dependency words parallel to its pinned Node layout.
// The scanner and lexer populate Reads during parsing; edits consume immutable
// per-subtree byte counts, without rerunning lexical decisions.
func legacyReuseReadsEligible(d *dfaTokenSource, source []byte) bool {
	if !legacyReuseReadHistoryEligible(d, source) {
		return false
	}
	if d.hasExternalSymbols || d.hasExternalScanner || d.language.ExternalScanner != nil {
		scanner, ok := d.language.ExternalScanner.(StatelessExternalScanner)
		stateless := ok && scanner.ExternalScannerIsStateless()
		if reads, ok := d.language.ExternalScanner.(incr.StatelessReadScanner); ok {
			stateless = stateless || reads.SupportsStatelessReadDependencies()
		}
		reads, checkpointReadCertified := d.language.ExternalScanner.(incr.CheckpointReadScanner)
		checkpointed := checkpointReadCertified && reads.SupportsCheckpointReadDependencies() &&
			languageUsesExternalScannerCheckpoints(d.language) && languageSupportsCheckpointedNonLeafReuse(d.language)
		if !d.hasExternalScanner || (!stateless && !checkpointed) {
			return false
		}
	}
	return true
}

func legacyReuseReadHistoryEligible(d *dfaTokenSource, source []byte) bool {
	if d == nil || !d.tokenInvariantInternalPrimitivesSupported() || d.lexer == nil || len(d.lexer.includedRanges) != 0 {
		return false
	}
	// Contextual close-angle probes currently have only aggregate read history.
	// Until they supply individual frontiers, keep their established verifier.
	return !supportsCompactCloseAngleSplit(d.language.Name) || !bytes.Contains(source, []byte(">>"))
}

func (a *nodeArena) beginLegacyReuseReads(d *dfaTokenSource, source []byte) {
	// Exact checkpoints authenticate scanner state at reused boundaries. The
	// read observer independently declines backward and column dependencies;
	// incomplete history keeps the fresh-verification route.
	if !legacyReuseReadsEligible(d, source) {
		return
	}
	if a.legacyReuseReads == nil {
		const cost = 64
		if !a.canAllocateLegacyReuseDependency(cost) {
			return
		}
		a.legacyReuseReads = incr.NewReads(len(source))
		if a.legacyReuseReads == nil {
			return
		}
		a.allocatedBytes += a.legacyReuseReads.Bytes()
	} else if !a.legacyReuseReads.Reset(len(source)) {
		return
	}
	a.legacyReuseReads.BindBudget(a.budgetBytes, a.budgetBaselineBytes, &a.allocatedBytes)
	d.lexer.reuseReads = a.legacyReuseReads
}

func (a *nodeArena) canAllocateLegacyReuseDependency(cost int64) bool {
	used := max(int64(0), a.allocatedBytes-a.budgetBaselineBytes)
	return cost >= 0 && (a.budgetBytes <= 0 || (used < a.budgetBytes && cost <= a.budgetBytes-used))
}

func (a *nodeArena) legacyReuseDependencyBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	total := a.legacyReuseReads.Bytes() + int64(cap(a.nodeReuseLookahead))*4
	if len(a.legacyReuseRawSymbols) != 0 {
		total += 256 + int64(len(a.legacyReuseRawSymbols))*96
	}
	for i := range a.nodeSlabs {
		total += int64(cap(a.nodeSlabs[i].reuseLookahead)) * 4
	}
	return total
}

func (a *nodeArena) resetLegacyReuseDependencies() {
	a.legacyReuseRawSymbols = nil
	if a.legacyReuseReads != nil {
		a.legacyReuseReads.Reset(-1)
		a.legacyReuseReads.TrimCapacity(maxRetainedChildSliceCapacityForClass(a.class))
	}
	a.legacyReuseDependenciesReady = false
	a.legacyReuseSourceChanged = false
	clear(a.nodeReuseLookahead)
	for i := range a.nodeSlabs {
		clear(a.nodeSlabs[i].reuseLookahead)
	}
}

func (a *nodeArena) prepareLegacyReuseDependencies() {
	if a == nil || a.legacyReuseReads == nil || a.legacyReuseDependenciesReady {
		return
	}
	a.legacyReuseReads.Seal()
	a.legacyReuseDependenciesReady = true
	parentReads := a.legacyReuseReads.Cursor(true)
	leafReads := a.legacyReuseReads.Cursor(false)
	fill := func(nodes []Node, used int, words *[]uint32) {
		if cap(*words) >= len(nodes) {
			*words = (*words)[:len(nodes)]
		} else {
			cost := int64(len(nodes)-cap(*words)) * 4
			if !a.canAllocateLegacyReuseDependency(cost) {
				return
			}
			*words = make([]uint32, len(nodes))
			a.allocatedBytes += cost
		}
		for i := 0; i < min(used, len(nodes)); i++ {
			var count uint32
			var ok bool
			if (*words)[i]&legacyReuseLeafKnown != 0 {
				count, ok = leafReads.Lookahead(nodes[i].endByte)
			} else {
				count, ok = parentReads.Lookahead(nodes[i].endByte)
			}
			if ok {
				if encoded := incr.Encode(count); encoded != 0 && encoded <= legacyReuseCountMask {
					(*words)[i] = (*words)[i]&^legacyReuseCountMask | encoded
				}
			}
		}
	}
	fill(a.nodes, a.used, &a.nodeReuseLookahead)
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		fill(slab.data, slab.used, &slab.reuseLookahead)
	}
}

func legacyReuseLookahead(n *Node) (uint32, bool) {
	if n == nil || n.ownerArena == nil || n.ownerArena.legacyReuseSourceChanged {
		return 0, false
	}
	if word := legacyReuseWord(n, false); word != nil {
		return incr.Decode(*word & legacyReuseCountMask)
	}
	return 0, false
}

// A whole-tree lexical shortcut proves today's token tuples, but does not
// rebuild each subtree's new scan frontier. Never carry the old bounds into
// a later structural reuse proof. Shared arenas abstain until their reset.
func (t *Tree) abstainLegacyReuseDependencies() {
	if t == nil {
		return
	}
	if t.arena != nil {
		t.arena.legacyReuseSourceChanged = true
	}
	for _, a := range t.borrowedArena {
		if a != nil {
			a.legacyReuseSourceChanged = true
		}
	}
}

func legacyReuseWord(n *Node, write bool) *uint32 {
	if n == nil || n.ownerArena == nil {
		return nil
	}
	a := n.ownerArena
	const size = unsafe.Sizeof(Node{})
	target := uintptr(unsafe.Pointer(n))
	get := func(nodes []Node, words *[]uint32) (*uint32, bool) {
		if len(nodes) == 0 {
			return nil, false
		}
		base := uintptr(unsafe.Pointer(&nodes[0]))
		if target < base || target >= base+uintptr(len(nodes))*size {
			return nil, false
		}
		i := int((target - base) / size)
		if write && i >= len(*words) {
			if cap(*words) >= len(nodes) {
				*words = (*words)[:len(nodes)]
			} else {
				cost := int64(len(nodes)-cap(*words)) * 4
				if !a.canAllocateLegacyReuseDependency(cost) {
					return nil, true
				}
				next := make([]uint32, len(nodes))
				copy(next, *words)
				*words = next
				a.allocatedBytes += cost
			}
		}
		if i >= len(*words) {
			return nil, true
		}
		return &(*words)[i], true
	}
	if word, found := get(a.nodes, &a.nodeReuseLookahead); found {
		return word
	}
	for i := range a.nodeSlabs {
		s := &a.nodeSlabs[i]
		if word, found := get(s.data, &s.reuseLookahead); found {
			return word
		}
	}
	return nil
}

func noteLegacyReuseLeaf(n *Node, tok Token) {
	if n == nil || n.ownerArena == nil || !n.ownerArena.legacyReuseReads.Recording() {
		return
	}
	if word := legacyReuseWord(n, true); word != nil {
		*word |= legacyReuseLeafKnown
		if tok.isKeyword() {
			*word |= legacyReuseKeyword
		}
	}
}

func (t *Tree) prepareLegacyReuseDependencies() {
	if t == nil {
		return
	}
	t.arena.prepareLegacyReuseDependencies()
	for _, a := range t.borrowedArena {
		if a != t.arena {
			a.prepareLegacyReuseDependencies()
		}
	}
}

// C edits invalidate lookahead dependencies even when a node's visible text
// ends before the edit. Preserve that text's coordinates and propagate changes
// down to every child whose recorded scan touched the edit.
func editLegacyLookaheadOnly(n *Node, edit InputEdit) bool {
	if n == nil || n.isMissing() || n.hasError() || n.endByte > edit.StartByte {
		return false
	}
	if n.endByte == edit.StartByte && n.startByte == n.endByte {
		return false
	}
	count, ok := legacyReuseLookahead(n)
	if !ok || uint64(n.endByte)+uint64(count) < uint64(edit.StartByte) {
		return false
	}
	if inputEditIsNoop(edit) && uint64(n.endByte)+uint64(count) == uint64(edit.StartByte) {
		return false
	}
	n.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	for i := 0; i < nodeChildCountNoMaterialize(n); i++ {
		child := nodeChildAtForReason(n, i, materializeForEdit)
		editLegacyLookaheadOnly(child, edit)
	}
	return true
}

func (p *Parser) legacyCanReuseFirstLeaf(state StateID, n *Node) bool {
	leaf := leftmostLeaf(n)
	if leaf == nil || int(state) >= len(p.language.LexModes) || int(leaf.preGotoState) >= len(p.language.LexModes) {
		return false
	}
	current, original := p.language.LexModes[state], p.language.LexModes[leaf.preGotoState]
	keyword := false // C nonterminal subtrees do not carry the keyword bit.
	if nodeChildCountNoMaterialize(n) == 0 {
		word := legacyReuseWord(n, false)
		keyword = word == nil || *word&legacyReuseLeafKnown == 0 || *word&legacyReuseKeyword != 0
	}
	rawSymbol := legacyReuseFirstLeafSymbol(leaf)
	entry := p.lookupAction(state, rawSymbol)
	hasActions, reusable := entry != nil && len(entry.Actions) != 0, entry != nil && entry.Reusable
	return incr.FirstLeaf(current.LexStateIndex() == noLookaheadLexState, hasActions,
		current == original, rawSymbol == p.language.KeywordCaptureToken,
		keyword, n.preGotoState == state,
		n.startByte == n.endByte && rawSymbol != 0, current.ExternalLexState != 0, reusable)
}

// The legacy tree omits C's leading-padding ownership. Its token source has
// already scanned the boundary, so authenticate that omitted boundary with the
// fresh token before applying the C table and lex-mode conditions.
func legacyReuseMatchesLookahead(n *Node, lookahead Token) bool {
	leaf := leftmostLeaf(n)
	return leaf != nil && legacyReuseFirstLeafSymbol(leaf) == lookahead.Symbol && leaf.startByte == lookahead.StartByte && leaf.endByte == lookahead.EndByte
}

// A clean terminal already lexed for a single ordinary shift skips no parser
// decision or input. Scanner-backed tokens still require their old receipts.
func (p *Parser) legacyCanReuseFreshLeaf(state StateID, n *Node, tok Token, ts TokenSource) bool {
	d := underlyingDFATokenSource(ts)
	if d == nil || d.hasExternalScanner || d.hasExternalSymbols || p.language.ExternalScanner != nil || n == nil || n.ChildCount() != 0 {
		return false
	}
	entry := p.lookupAction(state, n.Symbol())
	single := entry != nil && len(entry.Actions) == 1 && entry.Actions[0].Type == ParseActionShift
	return incr.FreshLeaf(single, legacyReuseMatchesLookahead(n, tok), single && entry.Actions[0].Extra == n.IsExtra(),
		n.PreGotoState() == state && !n.dirty() && !n.isFragile() && !n.HasError() && !n.IsMissing() && !n.IsError() && !tok.Missing && !tok.NoLookahead && n.dynamicPrecedence == 0, n.EndByte() > n.StartByte())
}
