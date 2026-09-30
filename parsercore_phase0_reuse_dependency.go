//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	"github.com/odvcencio/gotreesitter/internal/incr/dependency"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// A frontier includes all lexer probes and the actual reduction lookahead.
// Zero means unknown. The lexer records even EOF one byte past its cursor.
type compactReuseDependencies struct {
	ends     []uint32
	frontier uint32
	disabled bool
}

// Keep at most 64 KiB of pointer-free scratch per runner. Clear every entry
// before another parse can authenticate payloads with reused numeric IDs.
const compactReuseDependencyRetainedEntries = 16 * 1024

func (d *compactReuseDependencies) reset() compactReuseDependencies {
	if cap(d.ends) > compactReuseDependencyRetainedEntries {
		return compactReuseDependencies{}
	}
	clear(d.ends[:cap(d.ends)])
	return compactReuseDependencies{ends: d.ends[:0]}
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

// Publish top-level and nested candidates. Both need the original reduction's
// lookahead frontier before an edit can authenticate their right boundary.
// Borrowed nodes keep their original receipts and their original arena owner.
func (s *diagnosticParserCoreGenericScheduler) publishCompactReuseDependencies(
	p *Parser, root *Node, arena *nodeArena, nodesByID []*Node,
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
	// Publication owns only new-arena nodes. Serialize drafts with edits, while
	// readers can inspect published receipts without taking this lock.
	arena.compactReuseDependencyMu.Lock()
	defer arena.compactReuseDependencyMu.Unlock()
	// Interrupted publication must not supply proof state to a later attempt.
	defer arena.compactReuseDependencies.RangeStaged(arena.compactReuseDependencies.Clear)
	arena.compactReuseDependencyIndexed = false
	check := func(growth uint64) error {
		if err := poll(); err != nil {
			return err
		}
		additional := arenaAllocatedVolume(arena)
		for _, bytes := range []uint64{otherScratchBytes, growth} {
			if math.MaxUint64-additional < bytes {
				additional = math.MaxUint64
			} else {
				additional += bytes
			}
		}
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(additional); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		return nil
	}
	if err := check(0); err != nil {
		return err
	}
	stage := func(node *Node) error {
		slot, ok := arena.compactReuseDependencySlot(node)
		if !ok {
			return nil
		}
		if err := check(uint64(arena.compactReuseDependencies.Growth(slot))); err != nil {
			return err
		}
		cost, ok := arena.compactReuseDependencies.Stage(slot, dependency.Record{Start: node.startByte, End: node.endByte, Column: node.startPoint.Column}, arena.compactReuseDependencyBudget())
		if !ok {
			arena.compactReuseDependencies.Clear(slot)
		} else {
			arena.allocatedBytes += cost
		}
		return nil
	}
	for _, item := range root.children {
		if item == nil {
			continue
		}
		if eligible(item) {
			if err := stage(item); err != nil {
				return err
			}
		}
		for _, node := range item.children {
			if eligible(node) {
				if err := stage(node); err != nil {
					return err
				}
			}
			visited++
			if visited&255 == 0 {
				if err := check(0); err != nil {
					return err
				}
			}
		}
	}
	for id, node := range nodesByID {
		slot, owned := arena.compactReuseDependencySlot(node)
		if owned && arena.compactReuseDependencies.IsStaged(slot) {
			known := id != 0 && id < len(s.reuseDependencies.ends) && s.reuseDependencies.ends[id] >= node.EndByte() && viewFor != nil && points != nil
			if known {
				// Pointer identity does not authenticate compatibility rewrites. Require
				// the raw range and source coordinates for every collapsed projection.
				view, err := viewFor(core.SubtreeID(id))
				known = err == nil && view.StartByte == node.StartByte() && view.EndByte == node.EndByte() && points.point(node.StartByte()) == node.StartPoint() && points.point(node.EndByte()) == node.EndPoint()
			}
			frontier := uint32(0)
			if known {
				frontier = s.reuseDependencies.ends[id]
			}
			arena.compactReuseDependencies.Observe(slot, frontier, known)
		}
		if id&255 == 0 {
			if err := check(0); err != nil {
				return err
			}
		}
	}
	// Unknown and unseen producers cannot revive copied receipts. A successful
	// draft already contains the maximum authenticated extent at its final slot.
	var finishErr error
	arena.compactReuseDependencies.RangeStaged(func(slot int) {
		if finishErr != nil {
			return
		}
		arena.compactReuseDependencies.Finish(slot)
		visited++
		if visited&255 == 0 {
			finishErr = check(0)
		}
	})
	if finishErr != nil {
		return finishErr
	}
	return check(0)
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
