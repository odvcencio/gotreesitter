//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"math"

	"github.com/odvcencio/gotreesitter/internal/incr"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// A frontier includes all lexer probes and the actual reduction lookahead.
// Zero means unknown. The lexer records even EOF one byte past its cursor.
type compactReuseDependencies struct {
	reads *incr.Reads
	// idleReads owns reusable storage only. A non-nil reads marks an active
	// producer, so an ineligible parse must never inherit the idle pointer.
	idleReads      *incr.Reads
	readsAllocated int64
	leafWords      []uint32
	ends           []uint32
	frontier       uint32
	disabled       bool
}

// Keep at most 64 KiB of dependency scratch per runner. Clear every indexed
// receipt and invalidate the read history before another parse can use them.
const compactReuseDependencyRetainedEntries = 16 * 1024

func (d *compactReuseDependencies) reset() compactReuseDependencies {
	if d.reads != nil {
		// A larger input must not occupy the entire shared allowance forever.
		// Only an active producer's lengths express demand: the second reset
		// at scheduler initialization sees empty but useful idle buffers.
		d.ends = incr.RetainDependencyScratchForDemand(d.ends, 128)
		d.leafWords = incr.RetainDependencyScratchForDemand(d.leafWords, 128)
	}
	ends, leafWords := incr.ResetDependencyScratch(d.ends, d.leafWords, compactReuseDependencyRetainedEntries)
	reads := d.reads
	if reads == nil {
		reads = d.idleReads
	}
	if reads != nil {
		reads.Reset(-1)
		remaining := int64(compactReuseDependencyRetainedEntries-cap(ends)-cap(leafWords)) * 4
		const readHeaderBytes = 64
		if remaining < readHeaderBytes {
			reads = nil
		} else {
			reads.TrimCapacity(int((remaining - readHeaderBytes) / 8))
		}
	}
	return compactReuseDependencies{ends: ends, leafWords: leafWords, idleReads: reads}
}

func (d *compactReuseDependencies) invalidate() {
	if !d.disabled {
		clear(d.ends)
		d.disabled = true
	}
}

func (s *diagnosticParserCoreGenericScheduler) endCompactReuseDependency(before uint32, active bool, result *error) {
	if failure := recover(); failure != nil {
		s.reuseDependencies.invalidate()
		panic(failure)
	}
	err := s.finishCompactReuseDependency(before, active, *result == nil)
	if *result == nil {
		*result = err
	}
}

func (s *diagnosticParserCoreGenericScheduler) beginCompactReuseDependency(token Token) (uint32, bool) {
	d := &s.reuseDependencies
	if d.disabled {
		return 0, false
	}
	if !s.tokenSource.compactReuseForwardDependenciesOnly() || s.tokenSource.lexer == nil || len(s.headers) != 1 ||
		s.versionLexerOwnershipActive || s.recoveryIsolation || s.s3RegionOpened ||
		s.headers[0].isRecoveryLineage() || s.headers[0].recoveryRegion() != nil ||
		token.Missing || token.NoLookahead || token.EndByte < token.StartByte ||
		(token.ExternalScannerToken && !s.tokenSource.hasExternalScanner) ||
		(token.Symbol != 0 && !token.ExternalScannerToken && (!token.lexerInternalDFALexed() || token.EndByte <= token.StartByte)) ||
		token.lexerLookaheadEndByte == 0 {
		d.invalidate()
		return 0, false
	}
	// Mid-source zero-width scans carry loop-prevention history. A true EOF
	// sentinel cannot be borrowed: its examined frontier lies beyond source.
	if token.ExternalScannerToken && token.EndByte == token.StartByte &&
		(uint64(token.EndByte) != uint64(len(s.tokenSource.lexer.source)) || token.lexerLookaheadEndByte <= token.EndByte) {
		d.invalidate()
		return 0, false
	}
	exactPaths, err := s.compact.HeadExactPathCount(s.headers[0].head)
	subtrees := s.compact.SubtreeCount()
	if err != nil || exactPaths != 1 || uint64(subtrees)+1 != uint64(max(1, len(d.ends))) {
		d.invalidate()
		return 0, false
	}
	// A C frontier can stop at a valid rune's first byte. A dependency
	// receipt must also include the continuation bytes used to decode it.
	examinedEnd := tokenInvariantExaminedEnd(s.tokenSource.lexer.source, tokenLookaheadEndByte(token))
	d.frontier = maxUint32(d.frontier, examinedEnd)
	return uint32(subtrees), true
}

func (s *diagnosticParserCoreGenericScheduler) finishCompactReuseDependency(before uint32, active bool, success bool) error {
	d := &s.reuseDependencies
	if !active {
		return nil
	}
	if !success || len(s.headers) != 1 || s.versionLexerOwnershipActive || s.recoveryIsolation {
		d.invalidate()
		return nil
	}
	subtrees := uint32(s.compact.SubtreeCount())
	id, err := s.compact.SoleHeadPayload(s.headers[0].head)
	if err != nil || subtrees < before || id == 0 {
		d.invalidate()
		return nil
	}
	if err := s.growCompactReuseDependencies(subtrees); err != nil {
		return err
	}
	for fresh := uint64(before) + 1; fresh <= uint64(subtrees); fresh++ {
		d.ends[fresh] = d.frontier
		if fresh&255 == 0 {
			if err := s.pollStopControl(); err != nil {
				return err
			}
		}
	}
	// A cached reduction may republish an existing payload. Broaden its proof.
	if uint64(id) >= uint64(len(d.ends)) {
		return errors.New("compact reuse dependency payload is outside storage")
	}
	d.ends[id] = maxUint32(d.ends[id], d.frontier)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) growCompactReuseDependencies(last uint32) error {
	d := &s.reuseDependencies
	want := uint64(last) + 1
	if want > uint64(math.MaxInt) {
		return errors.New("compact reuse dependency storage overflow")
	}
	if want > uint64(cap(d.ends)) {
		capacity := max(want, uint64(cap(d.ends))*2, 128)
		if capacity > uint64(math.MaxInt) {
			capacity = want
		}
		additional := (capacity - uint64(cap(d.ends))) * 4
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(additional); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		next := make([]uint32, int(want), int(capacity))
		copy(next, d.ends)
		d.ends = next
	} else {
		d.ends = d.ends[:int(want)]
	}
	return nil
}

// compactProjectionNodeSource lets receipt publication read native records
// directly instead of allocating a pointer slot for every raw subtree.
// Reference publication retains its original raw-ID indexed mapping.
type compactProjectionNodeSource struct {
	nodes   []*Node
	records *core.ProjectionPages[nativeStreamRecord]
}

func (s compactProjectionNodeSource) len() int {
	if s.records != nil {
		return s.records.Len()
	}
	return len(s.nodes)
}

func (s compactProjectionNodeSource) at(index int) (core.SubtreeID, *Node) {
	if s.records != nil {
		record := s.records.At(index)
		if record.metadata.source == 0 {
			return 0, nil
		}
		return record.metadata.source, record.node
	}
	return core.SubtreeID(index), s.nodes[index]
}

// Publish top-level and nested candidates. Both need the original reduction's
// lookahead frontier before an edit can authenticate their right boundary.
// Borrowed nodes keep their original receipts and their original arena owner.
func (s *diagnosticParserCoreGenericScheduler) publishCompactReuseDependencies(
	p *Parser, root *Node, arena *nodeArena, nodesByID []*Node,
	viewFor func(core.SubtreeID) (core.MaterializationSubtreeView, error), points *diagnosticParserCorePointIndex,
	otherScratchBytes uint64, poll func() error,
) error {
	return s.publishCompactReuseDependenciesFrom(p, root, arena, compactProjectionNodeSource{nodes: nodesByID}, viewFor, points, otherScratchBytes, poll)
}

func (s *diagnosticParserCoreGenericScheduler) publishCompactReuseDependenciesFrom(
	p *Parser, root *Node, arena *nodeArena, nodes compactProjectionNodeSource,
	viewFor func(core.SubtreeID) (core.MaterializationSubtreeView, error), points *diagnosticParserCorePointIndex,
	otherScratchBytes uint64, poll func() error,
) error {
	if root == nil || root.HasError() {
		return nil
	}
	eligible := func(node *Node) bool {
		return node != nil && node.ownerArena == arena && node.ChildCount() != 0 &&
			node.EndByte() > node.StartByte() && !node.IsExtra() && !node.HasError() &&
			!node.dirty() && !node.isFragile() && compactNodeMayBeReused(node) &&
			compactNodeStateProofAvailable(node) && node.isCompactMaterialized() &&
			uint32(node.symbol) >= p.language.TokenCount && p.isVisibleSymbol(node.symbol)
	}
	count := 0
	visited := 0
	for _, item := range root.children {
		if item == nil {
			continue
		}
		if eligible(item) {
			count++
			if s.reuseDependencies.disabled {
				clearCompactReuseDependency(item)
			}
		}
		for _, node := range item.children {
			if eligible(node) {
				count++
				if s.reuseDependencies.disabled {
					// A compatibility clone can carry a prior receipt. An unknown
					// new derivation cannot authenticate that copied projection.
					clearCompactReuseDependency(node)
				}
			}
			visited++
			if visited&255 == 0 {
				if err := poll(); err != nil {
					return err
				}
			}
		}
	}
	if count == 0 || s.reuseDependencies.disabled {
		return nil
	}
	// Charge the transient pointer map before allocation. This conservative
	// estimate includes entries, spare buckets, and the map header.
	if uint64(count) > (math.MaxUint64-256)/64 {
		return errors.New("compact reuse publication storage overflow")
	}
	mapBytes := uint64(count)*64 + 256
	if math.MaxUint64-mapBytes < otherScratchBytes {
		mapBytes = math.MaxUint64
	} else {
		mapBytes += otherScratchBytes
	}
	check := func() error {
		if err := poll(); err != nil {
			return err
		}
		additional := arenaAllocatedVolume(arena)
		if math.MaxUint64-additional < mapBytes {
			additional = math.MaxUint64
		} else {
			additional += mapBytes
		}
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(additional); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	type proof struct {
		end     uint32
		seen    bool
		unknown bool
	}
	candidates := make(map[*Node]proof, count)
	for _, item := range root.children {
		if item == nil {
			continue
		}
		if eligible(item) {
			candidates[item] = proof{}
		}
		for _, node := range item.children {
			if eligible(node) {
				candidates[node] = proof{}
			}
			visited++
			if visited&255 == 0 {
				if err := check(); err != nil {
					return err
				}
			}
		}
	}
	for index := 0; index < nodes.len(); index++ {
		id, node := nodes.at(index)
		if candidate, ok := candidates[node]; ok {
			candidate.seen = true
			if id == 0 || uint64(id) >= uint64(len(s.reuseDependencies.ends)) || s.reuseDependencies.ends[id] < node.EndByte() ||
				viewFor == nil || points == nil {
				candidate.unknown = true
			} else {
				// Pointer identity does not authenticate compatibility rewrites.
				// Require the original byte range and exact source coordinates.
				view, err := viewFor(core.SubtreeID(id))
				if err != nil || view.StartByte != node.StartByte() || view.EndByte != node.EndByte() ||
					points.point(node.StartByte()) != node.StartPoint() || points.point(node.EndByte()) != node.EndPoint() {
					candidate.unknown = true
				} else {
					candidate.end = maxUint32(candidate.end, s.reuseDependencies.ends[id])
				}
			}
			candidates[node] = candidate
		}
		if index&255 == 0 {
			if err := check(); err != nil {
				return err
			}
		}
	}
	for node, candidate := range candidates {
		// A collapsed outer projection must not inherit an inner proof when
		// its own dependency is unknown. Unmatched alias clones also abstain.
		if !candidate.seen || candidate.unknown || !setCompactReuseDependency(node, candidate.end-node.EndByte()) {
			clearCompactReuseDependency(node)
		}
		visited++
		if visited&255 == 0 {
			if err := check(); err != nil {
				return err
			}
		}
	}
	return check()
}

func (s *compactIncrementalReuseSession) dependencyUnchanged(node *Node) bool {
	// Tree.Edit revokes receipts for nodes shifted by an earlier edit. A
	// trailing top-level sibling remains sound when its complete suffix is
	// byte-identical, including any lookahead and the EOF boundary.
	if s.cursor.topLevelSiblingBlockSpliceEligible(node) &&
		node.StartByte() >= s.cursor.topLevelResumeByte &&
		!s.cursor.rightBoundaryTouchedByEdit(uint32(len(s.cursor.newSource))) &&
		s.cursor.nodeBytesUnchanged(node.StartByte(), uint32(len(s.cursor.newSource))) {
		oldEnd, mapped := s.cursor.oldByteForNew(uint32(len(s.cursor.newSource)))
		if mapped && oldEnd == uint32(len(s.cursor.oldSource)) {
			return true
		}
	}
	bytes, ok := compactReuseDependencyForNode(node)
	end := uint64(node.EndByte()) + uint64(bytes)
	if !ok {
		return false
	}
	if end <= uint64(len(s.cursor.newSource)) {
		return !s.cursor.rightBoundaryTouchedByEdit(uint32(end)) &&
			s.cursor.nodeBytesUnchanged(node.StartByte(), uint32(end))
	}
	// Unmapped EOF and invalid UTF-8 sentinels remain dependencies.
	return false
}

func (s *diagnosticParserCoreGenericScheduler) importCompactReuseDependency(id core.SubtreeID, node *Node) error {
	d := &s.reuseDependencies
	if d.disabled {
		return nil
	}
	bytes, ok := compactReuseDependencyForNode(node)
	end := uint64(node.EndByte()) + uint64(bytes)
	if !ok || end > math.MaxUint32 || uint64(id) != uint64(max(1, len(d.ends))) {
		d.invalidate()
		return nil
	}
	if err := s.growCompactReuseDependencies(uint32(id)); err != nil {
		return err
	}
	d.frontier = maxUint32(d.frontier, maxUint32(uint32(end), tokenLookaheadEndByte(s.token)))
	d.ends[id] = d.frontier
	return nil
}

// Observe all elected and speculative lexer reads, including probes owned by
// different GLR heads. The sorted history remains conservative across restores.
func (s *diagnosticParserCoreGenericScheduler) beginCompactLegacyReads() {
	// Only publication with replay can stamp reusable public nodes. A known
	// no-replay request cannot consume this history; low-level scheduler
	// callers without a materialization context keep the conservative path.
	if s.options.materializationContextSet && !s.options.materializationForceReplayParseStates {
		s.reuseDependencies.idleReads = nil
		return
	}
	d := s.tokenSource
	if s.options.compactIncrementalReuse != nil || d == nil || d.lexer == nil || !legacyReuseReadsEligible(d, d.lexer.source) || !d.compactReuseForwardDependenciesOnly() {
		// This request cannot use the history. Drop its idle allocation before
		// the first stop-control poll charges the request's footprint.
		s.reuseDependencies.idleReads = nil
		return
	}
	reads := s.reuseDependencies.idleReads
	s.reuseDependencies.idleReads = nil
	if reads == nil {
		reads = incr.NewReads(len(d.lexer.source))
	} else if !reads.Reset(len(d.lexer.source)) {
		return
	}
	if reads == nil {
		return
	}
	limit := int64(min(s.options.stopControlMemoryBudgetBytes, s.options.stopControlHardCeilingBytes))
	if limit <= 0 {
		limit = int64(max(s.options.stopControlMemoryBudgetBytes, s.options.stopControlHardCeilingBytes))
	}
	used := int64(diagnosticParserCoreSchedulerFootprintBytes(s))
	// The idle pointer has been detached, so used excludes this history.
	// A previous larger input must not consume a smaller request's budget
	// merely because the runner kept its allocation between requests.
	if limit > 0 && reads.Bytes() >= limit-used {
		reads.TrimCapacity(0)
	}
	s.reuseDependencies.readsAllocated = used + reads.Bytes()
	if limit > 0 && s.reuseDependencies.readsAllocated >= limit {
		return
	}
	reads.BindBudget(limit, 0, &s.reuseDependencies.readsAllocated)
	s.reuseDependencies.reads = reads
	d.lexer.reuseReads = reads
}

// Replay stamps authenticate parser states; lexer history independently
// authenticates every original projection's byte dependencies. Unknown outer
// aliases cannot inherit a collapsed inner production's receipt.
func (s *diagnosticParserCoreGenericScheduler) publishCompactLegacyReads(arena *nodeArena, nodes []*Node, geometryFor func(core.SubtreeID) (core.SubtreeGeometry, error), points *diagnosticParserCorePointIndex, poll func() error) error {
	return s.publishCompactLegacyReadsFrom(arena, compactProjectionNodeSource{nodes: nodes}, geometryFor, points, poll)
}

func (s *diagnosticParserCoreGenericScheduler) publishCompactLegacyReadsFrom(arena *nodeArena, nodes compactProjectionNodeSource, geometryFor func(core.SubtreeID) (core.SubtreeGeometry, error), points *diagnosticParserCorePointIndex, poll func() error) error {
	reads := s.reuseDependencies.reads
	if reads == nil || arena == nil || geometryFor == nil || points == nil || s.s3RegionOpened || s.recoveryIsolation {
		return nil
	}
	reads.Seal()
	if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(arenaAllocatedVolume(arena)); resultMaterializationShouldStop(reason) {
		return diagnosticParserCoreStopControlTripped(reason)
	}
	// Several compact IDs can project onto one collapsed public node. Mark
	// every mismatch before publishing any receipt, so an aligned inner ID
	// cannot authenticate a mismatched outer projection. These nodes belong
	// exclusively to the unpublished arena; no borrowed node is marked.
	const rejected = nodeFlags(incr.TransientProjectionRejected)
	processed := 0
	defer func() {
		for index := 0; index < processed; index++ {
			_, node := nodes.at(index)
			if node != nil && node.ownerArena == arena {
				node.flags &^= rejected
			}
		}
	}()
	for index := 0; index < nodes.len(); index++ {
		id, node := nodes.at(index)
		if node == nil || node.ownerArena != arena {
			continue
		}
		processed = index + 1
		view, err := geometryFor(core.SubtreeID(id))
		if err != nil || view.StartByte != node.StartByte() || view.EndByte != node.EndByte() || points.point(node.StartByte()) != node.StartPoint() || points.point(node.EndByte()) != node.EndPoint() {
			node.flags |= rejected
		}
		if index&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
	}
	// Allocation order is normally source order. Keep independent cursors for
	// the parent and leaf boundary rules; GLR backtracking uses their exact
	// binary-search fallback when a later projection ends earlier.
	parents, leaves := reads.Cursor(true), reads.Cursor(false)
	for index := 0; index < nodes.len(); index++ {
		id, node := nodes.at(index)
		if node == nil || node.ownerArena != arena || node.flags&rejected != 0 || !compactNodeStateProofAvailable(node) {
			continue
		}
		var count uint32
		var ok bool
		if node.ChildCount() == 0 {
			count, ok = leaves.Lookahead(node.EndByte())
		} else {
			count, ok = parents.Lookahead(node.EndByte())
		}
		if ok {
			if encoded := incr.Encode(count); encoded != 0 && encoded <= legacyReuseCountMask {
				if word := legacyReuseWord(node, true); word != nil {
					*word = encoded
					if node.ChildCount() == 0 {
						*word |= incr.LeafReceipt(s.reuseDependencies.leafWords, uint32(id))
					}
				}
			}
		}
		if index&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Native records contain each public occurrence once, in arena allocation
// order. Verify that order while combining alias authentication, byte/point
// checks, and lookahead publication. Any ownership failure aborts publication;
// the caller still owns the unpublished arena and releases it on error.
func (s *diagnosticParserCoreGenericScheduler) publishNativeProjectionReads(arena *nodeArena, scratch *nativeProjectionWorkspace, points *diagnosticParserCorePointIndex, poll func() error) error {
	reads := s.reuseDependencies.reads
	publish := reads != nil && !s.s3RegionOpened && !s.recoveryIsolation
	var parents, leaves incr.LookaheadCursor
	if publish {
		reads.Seal()
		parents, leaves = reads.Cursor(true), reads.Cursor(false)
	}
	if err := poll(); err != nil {
		return err
	}
	block, words := arena.nodes, &arena.nodeReuseLookahead
	slab, offset := 0, 0
	for index := 1; index < scratch.records.Len(); index++ {
		if index&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		for offset >= len(block) {
			if slab >= len(arena.nodeSlabs) {
				return fmt.Errorf("native receipt exceeds arena allocation order")
			}
			part := &arena.nodeSlabs[slab]
			block, words = part.data, &part.reuseLookahead
			slab++
			offset = 0
		}
		slot := offset
		offset++
		record := scratch.records.At(index)
		node := record.node
		if node != &block[slot] || node.ownerArena != arena {
			return fmt.Errorf("native receipt does not own its unique arena slot")
		}
		id := record.metadata.source
		leaf := node.ChildCount() == 0
		geometry, compatible, err := s.compact.NativeProjectionReceiptGeometry(id, core.Symbol(node.symbol), leaf, core.Symbol(record.metadata.field) == core.Symbol(node.symbol))
		if err != nil || !compatible {
			record.metadata.source = 0
			continue
		}
		if !publish || geometry.StartByte != node.startByte || geometry.EndByte != node.endByte || points.point(node.startByte) != node.startPoint || points.point(node.endByte) != node.endPoint || !compactNodeStateProofAvailable(node) {
			continue
		}
		var count uint32
		var ok bool
		if leaf {
			count, ok = leaves.Lookahead(node.endByte)
		} else {
			count, ok = parents.Lookahead(node.endByte)
		}
		if !ok {
			continue
		}
		encoded := incr.Encode(count)
		if encoded == 0 || encoded > legacyReuseCountMask {
			continue
		}
		word, growth := incr.PublicationWord(words, slot, len(block), arena.canAllocateLegacyReuseDependency)
		arena.allocatedBytes += growth
		if word != nil {
			*word = encoded
			if leaf {
				*word |= incr.LeafReceipt(s.reuseDependencies.leafWords, uint32(id))
			}
		}
	}
	return nil
}

// Each shift receipt preserves the actual elected token's keyword flag. A
// collapsed or aliased terminal does not inherit another token's provenance.
func (s *diagnosticParserCoreGenericScheduler) beginCompactLeafReceipt(tok Token) (uint32, Token) {
	return uint32(s.compact.SubtreeCount()), tok
}
func (s *diagnosticParserCoreGenericScheduler) endCompactLeafReceipt(before uint32, tok Token, result *error) {
	d := &s.reuseDependencies
	if *result != nil || d.reads == nil {
		return
	}
	last := uint32(s.compact.SubtreeCount())
	if last < before {
		d.reads.Abstain()
		return
	}
	want := uint64(last) + 1
	if want > uint64(math.MaxInt) {
		d.reads.Abstain()
		return
	}
	// Sixteen two-bit leaf receipts share each word.
	want = (want + 15) / 16
	if want > uint64(cap(d.leafWords)) {
		capacity := max(want, uint64(cap(d.leafWords))*2, 128)
		if capacity > uint64(math.MaxInt) {
			capacity = want
		}
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes((capacity - uint64(cap(d.leafWords))) * 4); resultMaterializationShouldStop(reason) {
			*result = diagnosticParserCoreStopControlTripped(reason)
			return
		}
		next := make([]uint32, int(want), int(capacity))
		copy(next, d.leafWords)
		d.leafWords = next
	} else {
		d.leafWords = d.leafWords[:int(want)]
	}
	// Preserve the original exact-ID bound after rollback truncates a word.
	incr.TrimLeafReceiptTail(d.leafWords, last)
	for id := uint64(before) + 1; id <= uint64(last); id++ {
		view, err := s.compact.SubtreeGeometry(core.SubtreeID(id))
		if err != nil {
			d.reads.Abstain()
			return
		}
		if view.Terminal && !view.Missing && Symbol(view.Symbol) == tok.Symbol && view.StartByte == tok.StartByte && view.EndByte == tok.EndByte {
			flags := legacyReuseLeafKnown
			if tok.isKeyword() {
				flags |= legacyReuseKeyword
			}
			incr.SetLeafReceipt(d.leafWords, uint32(id), flags)
		}
	}
}
