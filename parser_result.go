package gotreesitter

import (
	"fmt"
	"time"
)

// Parser-result assembly owns the private handoff from GLR/parse-stack nodes to
// the returned Tree. Runtime files named parser_result_*.go stay in this package
// because many compatibility normalizers rewrite private Node, Language, Symbol,
// and nodeArena state directly. Public-API parser-result regressions live in
// parser_result_test, while source fixtures belong under testdata.

// reconcileStaleHasErrorFlags repairs hasError bits that no longer match the
// subtree contents after C-recovery wrap/unwrap cycles. During recovery a
// stack's nodes legitimately carry hasError while an open ERROR region exists;
// when the region later resolves losslessly (the absorbed content re-reduces
// into ordinary productions and the ERROR wrapper is spliced away), ancestor
// flags set during the wrapped phase can be left behind. C cannot represent
// this state: ts_subtree_has_error is DERIVED (error_cost > 0, which only
// ERROR and MISSING subtrees contribute), so a tree with no ERROR/MISSING
// descendant is definitionally HasError=false. A stale root flag is not
// cosmetic — the retry ladder's treeParseClean/shouldRetryAcceptedErrorParse
// read it and will run every widened retry pass on an already-clean parse
// (bash cliff RCA 2026-07: 46s for a 657-byte file, ~7 wasted full passes).
//
// Clear-only by design: a node whose flag is set but whose subtree carries no
// ERROR/MISSING is repaired; under-set flags are left alone (some language
// normalizers deliberately leave repaired regions unflagged). Subtrees deeper
// than maxTreeWalkDepth keep their existing claim (never cleared unverified).
// Returns whether the subtree truly contains an error.
func reconcileStaleHasErrorFlags(n *Node, depth int) bool {
	if n == nil {
		return false
	}
	if n.symbol == errorSymbol || n.isMissing() {
		return true
	}
	if depth >= maxTreeWalkDepth {
		return n.hasError()
	}
	has := false
	// Materializing accessors: recovery-produced spines can still hold
	// unmaterialized child forms here, and a no-materialize count would skip
	// exactly the subtrees this repair exists for.
	for i, count := 0, nodeChildCount(n); i < count; i++ {
		// No early exit: every stale sibling flag gets repaired.
		if reconcileStaleHasErrorFlags(resultChildAt(n, i), depth+1) {
			has = true
		}
	}
	if !has && n.hasError() {
		n.setHasError(false)
	}
	return has
}

type parseMaterializationTiming struct {
	resultSelectionNanos               int64
	transientParentMaterializeNanos    int64
	resultTreeBuildNanos               int64
	transientChildMaterializationNanos int64
	pythonKeywordRepairNanos           int64
	pythonRootRepairNanos              int64
	resultFinalizeRootNanos            int64
	resultExtendTrailingNanos          int64
	resultNormalizeRootStartNanos      int64
	resultCompatibilityNanos           int64
	resultParentLinkNanos              int64
	reduceRangeNanos                   int64
	reducePendingParentNanos           int64
	reduceChildBuildNanos              int64
	reduceParentBuildNanos             int64
	reduceSpanNanos                    int64
	reduceStackPushNanos               int64
	reduceNoTreeBuildNanos             int64
	actionExtraShiftNanos              int64
	actionNoActionNanos                int64
	actionNoActionRelexNanos           int64
	actionNoActionMissingNanos         int64
	actionNoActionRecoverNanos         int64
	actionNoActionErrorNanos           int64
	actionConflictChoiceNanos          int64
	actionConflictForkNanos            int64
	actionSingleShiftNanos             int64
	actionSingleReduceNanos            int64
	actionSingleAcceptNanos            int64
	actionSingleRecoverNanos           int64
	actionSingleOtherNanos             int64
}

func materializationTimingStart(t *parseMaterializationTiming) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Now()
}

func (t *parseMaterializationTiming) addPythonKeywordRepair(start time.Time) {
	if t != nil {
		t.pythonKeywordRepairNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addPythonRootRepair(start time.Time) {
	if t != nil {
		t.pythonRootRepairNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultFinalizeRoot(start time.Time) {
	if t != nil {
		t.resultFinalizeRootNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultExtendTrailing(start time.Time) {
	if t != nil {
		t.resultExtendTrailingNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultNormalizeRootStart(start time.Time) {
	if t != nil {
		t.resultNormalizeRootStartNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultCompatibility(start time.Time) {
	if t != nil {
		t.resultCompatibilityNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultParentLink(start time.Time) {
	if t != nil {
		t.resultParentLinkNanos += time.Since(start).Nanoseconds()
	}
}

func (p *Parser) currentMaterializationTiming() *parseMaterializationTiming {
	if p == nil {
		return nil
	}
	return p.materializationTiming
}

func (p *Parser) resultMaterializationStopReason(arena *nodeArena) ParseStopReason {
	if p != nil {
		if reason := p.materializationParseStopReason(); parseStopReasonIsActive(reason) {
			return reason
		}
		// compatMemoryBudgetTripped is compat normalization's own sticky
		// latch, shared by the Go and JS/TS fused compat walks (see
		// (*Parser).compatRuntimeMemoryBudgetStopReason): unlike
		// arena.budgetExhausted() below, which stays true for the rest of the
		// parse once tripped, a runtime heap/sys budget trip is inherently
		// non-monotonic (GC can make heap growth appear to recede), so without
		// this latch a later caller re-checking here (e.g. finalizeTree, right
		// after Go compat normalization returns) could miss a trip Go compat
		// already acted on and stamp the tree with the wrong stop reason.
		if p.compatMemoryBudgetTripped {
			return ParseStopMemoryBudget
		}
	}
	if arena != nil && arena.budgetExhausted() {
		return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
	}
	if p != nil && p.budgetScratch != nil && p.budgetScratch.budgetExhausted() {
		// Closes the gap documented in
		// spore.2026-08-02.walnut-e.memory-exhaustion: gssScratch (the
		// exploding allocator in production's C-recovery missing-token
		// search, cHandleError/cDoAllPotentialReductions in
		// parser_recover_c.go) has no budget state of its own, but the
		// owning parserScratch already tracks it inside its own
		// allocatedBytes()/budgetExhausted() (parser_scratch.go) — the SAME
		// 512MB soft budget arena.budgetExhausted() above enforces, armed
		// unconditionally regardless of source length (parseMemoryBudget,
		// parser_limits.go). Before this check existed, the main GLR loop's
		// own per-token checkpoints (parser.go) called scratch.budgetExhausted()
		// directly, right beside their own resultMaterializationStopReason
		// call, because this function could not see it; those five direct
		// calls are gone now (this check makes them redundant), and every
		// OTHER resultMaterializationStopReason call site — including the
		// C-recovery candidate search, which never had a direct
		// scratch.budgetExhausted() check of its own — gets the same
		// coverage those five sites always had.
		return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceScratch)
	}
	if p != nil {
		if reason := p.runtimeMemoryBudgetStopReason(arenaAllocatedVolume(arena) + scratchAllocatedVolume(p.budgetScratch)); reason == ParseStopMemoryBudget {
			return reason
		}
	}
	return ParseStopNone
}

// arenaAllocatedVolume returns the arena's cheap, already-tracked allocated-bytes
// counter as the volume signal for the runtime memory poll's volume-triggered
// bypass (see runtimeMemoryBudgetStopReason). It costs no syscalls: allocatedBytes
// is updated incrementally at every arena materialization boundary.
func arenaAllocatedVolume(arena *nodeArena) uint64 {
	if arena == nil || arena.allocatedBytes <= 0 {
		return 0
	}
	return uint64(arena.allocatedBytes)
}

// scratchAllocatedVolume returns the active parse's parserScratch
// allocated-bytes counter — dominated in pathological cases by
// gssScratch.allocatedBytes (see budgetScratch's doc comment on Parser) — as a
// second, equally cheap volume signal alongside arenaAllocatedVolume. Costs no
// syscalls: parserScratch.allocatedBytes() sums already-tracked
// per-substructure counters, each updated incrementally at its own
// slab-allocation boundary (e.g. (*gssScratch).allocNodeSlow,
// glr_gss.go:629).
func scratchAllocatedVolume(scratch *parserScratch) uint64 {
	if scratch == nil {
		return 0
	}
	bytes := scratch.allocatedBytes()
	if bytes <= 0 {
		return 0
	}
	return uint64(bytes)
}

func resultMaterializationShouldStop(reason ParseStopReason) bool {
	return parseStopReasonIsActive(reason) || reason == ParseStopMemoryBudget || reason == ParseStopInvariantViolation
}

// hasCleanSiblingAtSamePosition reports whether some other live (non-dead)
// stack in stacks, besides self, reaches the same final byte position as
// stacks[self] without itself carrying an unvalidated C-recovery marker. Used
// by buildResultFromGLR as an extra safety check before trusting
// cRecoveryUnvalidatedMarker on the selected stack: if an independent,
// unflagged derivation also completed at the same position, the flagged
// stack's clean result is corroborated by a genuinely separate parse rather
// than being the sole survivor of a recovery-owning competitor's elimination.
func hasCleanSiblingAtSamePosition(stacks []glrStack, self int) bool {
	if self < 0 || self >= len(stacks) {
		return false
	}
	pos := stacks[self].byteOffset
	for i := range stacks {
		if i == self || stacks[i].dead {
			continue
		}
		if stacks[i].byteOffset != pos {
			continue
		}
		if !stacks[i].cRecoveryUnvalidatedMarker {
			return true
		}
	}
	return false
}

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries. When
// accepted stacks are otherwise tied, prefer the tree that retains an
// alias-target symbol, then the conservative tree-order tie-break before
// falling back to branch order.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node, transientParents *transientParentScratch, transientChildren *transientChildScratch, skipErrorRank bool, materializationTiming *parseMaterializationTiming) *Tree {
	errorTreeWithStopReason := func(reason ParseStopReason) *Tree {
		tree := parseErrorTreeWithArena(source, p.language, arena)
		tree.setParseStopReason(reason)
		return tree
	}
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		return errorTreeWithStopReason(reason)
	}
	if len(stacks) == 0 {
		return parseErrorTreeWithArena(source, p.language, arena)
	}
	workCountRecordFinalExpansions(p, stacks) // work-count-assembly: convergence final-expand seam
	stacks = expandPackedGSSResultPaths(stacks)
	p.emitRawShapeDiag("pre_result_selection", stacks, arena)
	selectionStart := time.Time{}
	if materializationTiming != nil {
		selectionStart = time.Now()
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if i&63 == 0 {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				return errorTreeWithStopReason(reason)
			}
		}
		cmp := stackCompareForResultSelection(p, arena, &stacks[i], &stacks[best], skipErrorRank)
		if p.glrTrace && p.errorCostCompetitionEnabled() {
			p.traceCResultSelectionCompare(i, best, cmp, &stacks[i], &stacks[best], arena)
		}
		if cmp > 0 {
			best = i
		}
	}
	if materializationTiming != nil {
		materializationTiming.resultSelectionNanos += time.Since(selectionStart).Nanoseconds()
	}
	selected := stacks[best]
	workCountRecordFinalSelect(p, stacks, &selected)
	// crecoveryDroppedErrorForClean is set ONLY here, and only for the stack
	// that is actually about to become the returned tree — never for a drop
	// or fork that happened on some other, discarded lineage elsewhere in the
	// parse. cRecoveryUnvalidatedMarker is carried on selected because it
	// itself created a real ERROR node via cRecoverToState (for a
	// single-stack dead end with a small recovered span — see
	// cRecoveryUnvalidatedMarker's doc comment in glr.go for why this is the
	// only trigger) and never cycled back through cHandleError to be
	// re-validated by another cost competition — cStackErrorCost cannot
	// legitimately have fallen back to zero in that case. The same-position
	// sibling check below is an additional guard: if another live, non-dead
	// stack reaches this exact position with no unvalidated marker of its
	// own, a genuinely independent clean derivation also completed here, so
	// the flagged lineage's clean result is corroborated rather than
	// suspicious, and resolveCRecoverySwallowedError is not worth the
	// discarded-reparse cost of double-checking it.
	if p.errorCostCompetitionEnabled() && selected.cRecoveryUnvalidatedMarker &&
		!hasCleanSiblingAtSamePosition(stacks, best) {
		p.crecoveryDroppedErrorForClean = true
	}
	if p.glrTrace && p.errorCostCompetitionEnabled() {
		p.traceCResultSelectionSelected(best, &selected, arena)
	}
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		return errorTreeWithStopReason(reason)
	}
	if len(selected.entries) > 0 {
		materializeStart := time.Time{}
		if materializationTiming != nil {
			materializeStart = time.Now()
		}
		if reason := materializeTransientParentEntries(selected.entries, arena, transientParents, transientChildren, p); resultMaterializationShouldStop(reason) {
			return errorTreeWithStopReason(reason)
		}
		if materializationTiming != nil {
			materializationTiming.transientParentMaterializeNanos += time.Since(materializeStart).Nanoseconds()
		}
		buildStart := time.Time{}
		if materializationTiming != nil {
			buildStart = time.Now()
		}
		tree := p.buildResult(selected.entries, source, arena, oldTree, reuseState, linkScratch)
		if materializationTiming != nil {
			materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
		}
		return tree
	}
	if selected.gss.head == nil {
		buildStart := time.Time{}
		if materializationTiming != nil {
			buildStart = time.Now()
		}
		tree := p.buildResult(nil, source, arena, oldTree, reuseState, linkScratch)
		if materializationTiming != nil {
			materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
		}
		return tree
	}
	nodes, reason := nodesFromGSSMaterializingCompactFullLeaves(p, selected.gss, arena)
	if resultMaterializationShouldStop(reason) {
		return errorTreeWithStopReason(reason)
	}
	materializeStart := time.Time{}
	if materializationTiming != nil {
		materializeStart = time.Now()
	}
	if reason := materializeTransientParentNodes(nodes, arena, transientParents, transientChildren, p); resultMaterializationShouldStop(reason) {
		return errorTreeWithStopReason(reason)
	}
	if materializationTiming != nil {
		materializationTiming.transientParentMaterializeNanos += time.Since(materializeStart).Nanoseconds()
	}
	buildStart := time.Time{}
	if materializationTiming != nil {
		buildStart = time.Now()
	}
	tree := p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
	if materializationTiming != nil {
		materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
	}
	return tree
}

func (p *Parser) traceCResultSelectionCompare(candidateIndex, bestIndex, cmp int, candidate, best *glrStack, arena *nodeArena) {
	fmt.Printf("      -> C-RESULT-CMP cand=%d %s best=%d %s cmp=%d\n",
		candidateIndex,
		p.cResultSelectionTraceSummary(candidate, arena),
		bestIndex,
		p.cResultSelectionTraceSummary(best, arena),
		cmp,
	)
}

func (p *Parser) traceCResultSelectionSelected(index int, selected *glrStack, arena *nodeArena) {
	fmt.Printf("      -> C-RESULT-SELECT index=%d %s\n", index, p.cResultSelectionTraceSummary(selected, arena))
}

func (p *Parser) cResultSelectionTraceSummary(s *glrStack, arena *nodeArena) string {
	if s == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{kind:%s accepted:%v state:%d byte:%d depth:%d score:%d cost:%d dyn:%d errRank:%d}",
		cRecoverStackTraceKind(s),
		s.accepted,
		s.top().state,
		s.byteOffset,
		s.depth(),
		s.score,
		p.cStackResultErrorCost(s),
		stackResultDynamicPrecedence(s),
		stackResultErrorRank(s, arena),
	)
}

func expandPackedGSSResultPaths(stacks []glrStack) []glrStack {
	var expanded []glrStack
	for i := range stacks {
		s := stacks[i]
		if len(s.entries) > 0 || s.gss.head == nil || !gssInlineChainHasPackedLinks(s.gss.head) {
			if expanded != nil {
				expanded = append(expanded, s)
			}
			continue
		}
		if expanded == nil {
			expanded = make([]glrStack, 0, len(stacks)+1)
			expanded = append(expanded, stacks[:i]...)
		}
		expanded = appendExpandedGSSResultPaths(expanded, s, maxStacksPerMergeKey)
	}
	if expanded == nil {
		return stacks
	}
	return expanded
}

func gssInlineChainHasPackedLinks(n *gssNode) bool {
	for ; n != nil; n = n.prev {
		if n.linkCount() > 1 {
			return true
		}
	}
	return false
}

func appendExpandedGSSResultPaths(dst []glrStack, source glrStack, capPerStack int) []glrStack {
	if capPerStack <= 0 {
		capPerStack = 1
	}
	startLen := len(dst)
	revPath := make([]stackEntry, 0, source.depth())
	var dfs func(*gssNode)
	dfs = func(n *gssNode) {
		if n == nil {
			if capPerStack == 0 {
				return
			}
			pathLen := len(revPath)
			entries := make([]stackEntry, pathLen)
			for i := 0; i < pathLen; i++ {
				entries[i] = revPath[pathLen-1-i]
			}
			candidate := source
			candidate.gss = gssStack{}
			candidate.entries = entries
			// The source aggregate describes its original contiguous path (if
			// any), not this newly materialized packed-link alternative.
			candidate.invalidateCEntryAgg()
			dst = append(dst, candidate)
			capPerStack--
			return
		}
		for i, count := 0, n.linkCount(); i < count && capPerStack > 0; i++ {
			prev, entry := n.link(i)
			revPath = append(revPath, entry)
			dfs(prev)
			revPath = revPath[:len(revPath)-1]
		}
	}
	dfs(source.gss.head)
	if len(dst) > startLen {
		return dst
	}
	return append(dst, source)
}

func materializeTransientParentEntries(entries []stackEntry, arena *nodeArena, transientParents *transientParentScratch, transientChildren *transientChildScratch, p *Parser) ParseStopReason {
	if transientParents == nil {
		return ParseStopNone
	}
	return transientParents.materializeEntriesUntil(entries, arena, transientChildren, p)
}

func materializeTransientParentEntriesForRecycle(entries []stackEntry, arena *nodeArena, transientParents *transientParentScratch, transientChildren *transientChildScratch, p *Parser) ParseStopReason {
	if transientParents == nil {
		return ParseStopNone
	}
	return transientParents.materializeEntriesForRecycleUntil(entries, arena, transientChildren, p)
}

func materializeTransientParentNodes(nodes []*Node, arena *nodeArena, transientParents *transientParentScratch, transientChildren *transientChildScratch, p *Parser) ParseStopReason {
	if transientParents == nil {
		return ParseStopNone
	}
	return transientParents.materializeNodeSliceUntil(nodes, arena, transientChildren, p)
}

func (p *Parser) buildNoTreeBenchmarkResult(source []byte, arena *nodeArena, rootEndByte uint32) *Tree {
	if arena == nil {
		return NewTree(nil, source, p.language)
	}
	sym := Symbol(0)
	if p != nil && p.hasRootSymbol {
		sym = p.rootSymbol
	}
	named := true
	if p != nil && p.language != nil {
		named = p.isNamedSymbol(sym)
	}
	root := arena.allocNodeFast()
	root.ownerArena = arena
	arena.noTreePlaceholderNodesConstructed++
	retagResultRoot(root, sym, named)
	root.startByte = 0
	root.endByte = rootEndByte
	root.childIndex = -1
	nodeInitEquivVersion(root)
	return newTreeWithArenas(root, source, p.language, arena, nil)
}

func stackCompareForResultSelection(p *Parser, arena *nodeArena, a, b *glrStack, skipErrorRank bool) int {
	return stackCompareForResultSelectionWithRawShape(p, arena, a, b, skipErrorRank, true)
}

func stackCompareForResultSelectionWithRawShape(p *Parser, arena *nodeArena, a, b *glrStack, skipErrorRank bool, useRawShape bool) int {
	if a.dead != b.dead {
		if a.dead {
			return -1
		}
		return 1
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if p != nil && p.errorCostCompetitionEnabled() {
		// Faithful C recovery port: ts_parser__select_tree picks the tree
		// with the lower error cost first, then the higher dynamic
		// precedence. If both candidates still tie and have nonzero error
		// cost, C selects the later candidate.
		if ac, bc := p.cStackResultErrorCost(a), p.cStackResultErrorCost(b); ac != bc {
			if ac < bc {
				return 1
			}
			return -1
		} else if aDyn, bDyn := stackResultDynamicPrecedence(a), stackResultDynamicPrecedence(b); aDyn != bDyn {
			if aDyn > bDyn {
				return 1
			}
			return -1
		} else if ac > 0 {
			return 1
		}
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if !skipErrorRank {
		if aErr, bErr := stackResultErrorRank(a, arena), stackResultErrorRank(b, arena); aErr != bErr {
			if aErr < bErr {
				return 1
			}
			return -1
		}
	}
	if aDyn, bDyn := stackResultDynamicPrecedence(a), stackResultDynamicPrecedence(b); aDyn != bDyn {
		if aDyn > bDyn {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if cmp := compareAcceptedStackAliasPreference(p, arena, a, b); cmp != 0 {
		return cmp
	}
	if cmp := compareAcceptedStackTreeOrderPreference(p, arena, a, b); cmp != 0 {
		return cmp
	}
	// The raw-shape/symbol-id fallback below is a last-resort ordering with no
	// grammar-authored basis: compareRawStackEntriesRec breaks ties on raw
	// numeric Symbol id, an artifact of grammar-compiler assignment order that
	// carries no relationship to which production the C oracle actually
	// prefers. A CompactPrimaryAcceptanceDerivationCertified language has
	// already proven (via the compact route's own primary-derivation election,
	// admission_switch_candidate.go) that C keeps whichever accepted
	// derivation never took a GLR-forked conflict branch; skipping the
	// symbol-id fallback for exactly these certified languages lets that same
	// signal -- already present below as the branchOrder tie-break, lower
	// (never-forked) wins -- decide instead, rather than losing to an
	// unrelated ordering first. This changes nothing for every uncertified
	// language.
	if useRawShape && (p == nil || p.language == nil || !p.language.CompactPrimaryAcceptanceDerivationCertified) {
		if cmp := compareAcceptedStackRawShapePreference(p, arena, a, b); cmp != 0 {
			return cmp
		}
	}
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.branchOrder != b.branchOrder {
		if a.branchOrder < b.branchOrder {
			return 1
		}
		return -1
	}
	return 0
}

// compareAcceptedStackTreeOrderPreference is a narrow final tie-break for
// accepted stacks that already match on error cost and dynamic precedence. It
// does not count descendants. It only compares candidates whose top-level
// result entries have identical envelopes, and it only prefers a sibling-spliced
// tree when both sides preserve a concrete shared prefix.
func compareAcceptedStackTreeOrderPreference(p *Parser, arena *nodeArena, a, b *glrStack) int {
	if !a.accepted || !b.accepted {
		return 0
	}
	aCount := stackMaterializingResultEntryCount(a)
	if aCount == 0 || aCount != stackMaterializingResultEntryCount(b) {
		return 0
	}
	const maxBufferedTreeOrderEntries = 8
	if aCount > maxBufferedTreeOrderEntries {
		return 0
	}
	var aBuf, bBuf [maxBufferedTreeOrderEntries]stackEntry
	aEntries, aOK := stackMaterializingResultEntries(a, aBuf[:0], aCount)
	bEntries, bOK := stackMaterializingResultEntries(b, bBuf[:0], aCount)
	if !aOK || !bOK {
		return 0
	}
	for i := 0; i < aCount; i++ {
		if !stackEntriesHaveSameTreeOrderEnvelope(aEntries[i], bEntries[i]) {
			return 0
		}
	}
	for i := 0; i < aCount; i++ {
		if cmp := compareStackEntryTreeOrder(p, arena, aEntries[i], bEntries[i], 0); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareStackEntryTreeOrder(p *Parser, arena *nodeArena, a, b stackEntry, depth int) int {
	if depth > maxTreeWalkDepth {
		return 0
	}
	if !stackEntriesHaveSameTreeOrderEnvelope(a, b) {
		return 0
	}
	aCount := stackEntryNodeChildCount(a)
	bCount := stackEntryNodeChildCount(b)
	limit := aCount
	if bCount < limit {
		limit = bCount
	}
	for i := 0; i < limit; i++ {
		aChild, aOK := stackEntryAliasChild(a, arena, i)
		bChild, bOK := stackEntryAliasChild(b, arena, i)
		if !aOK || !bOK {
			return 0
		}
		if !stackEntriesHaveSameTreeOrderEnvelope(aChild, bChild) {
			if stackEntryVisibleNamedUnaryWrapperContains(p, arena, aChild, bChild) ||
				stackEntryVisibleNamedUnaryWrapperContains(p, arena, bChild, aChild) {
				return 0
			}
			if cmp := compareStackEntryDirectChildPreference(p, arena, aChild, bChild, depth+1); cmp != 0 {
				return cmp
			}
			return compareStackEntrySharedPrefixSplice(p, arena, a, b, i, depth+1)
		}
		if cmp := compareStackEntryTreeOrder(p, arena, aChild, bChild, depth+1); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func stackEntriesHaveSameTreeOrderEnvelope(a, b stackEntry) bool {
	return stackEntryNodeSymbol(a) == stackEntryNodeSymbol(b) &&
		stackEntryNodeStartByte(a) == stackEntryNodeStartByte(b) &&
		stackEntryNodeEndByte(a) == stackEntryNodeEndByte(b) &&
		stackEntryNodeIsExtra(a) == stackEntryNodeIsExtra(b) &&
		stackEntryNodeIsMissing(a) == stackEntryNodeIsMissing(b) &&
		stackEntryNodeHasError(a) == stackEntryNodeHasError(b)
}

func compareStackEntryDirectChildPreference(p *Parser, arena *nodeArena, a, b stackEntry, depth int) int {
	if depth > maxTreeWalkDepth {
		return 0
	}
	if stackEntryVisibleNamedUnaryWrapperContains(p, arena, a, b) {
		return 1
	}
	if stackEntryVisibleNamedUnaryWrapperContains(p, arena, b, a) {
		return -1
	}
	if stackEntryUnaryWrapperContains(p, arena, a, b, depth+1) {
		return -1
	}
	if stackEntryUnaryWrapperContains(p, arena, b, a, depth+1) {
		return 1
	}
	return 0
}

func compareStackEntryVisibleNamedWrapperPreference(p *Parser, arena *nodeArena, a, b stackEntry, depth int) int {
	if depth > maxTreeWalkDepth || !stackEntriesHaveSameTreeOrderEnvelope(a, b) {
		return 0
	}
	aCount := stackEntryNodeChildCount(a)
	bCount := stackEntryNodeChildCount(b)
	if aCount != bCount {
		return 0
	}
	for i := 0; i < aCount; i++ {
		aChild, aOK := stackEntryAliasChild(a, arena, i)
		bChild, bOK := stackEntryAliasChild(b, arena, i)
		if !aOK || !bOK {
			return 0
		}
		if stackEntriesHaveSameTreeOrderEnvelope(aChild, bChild) {
			if cmp := compareStackEntryVisibleNamedWrapperPreference(p, arena, aChild, bChild, depth+1); cmp != 0 {
				return cmp
			}
			continue
		}
		if stackEntryVisibleNamedUnaryWrapperContains(p, arena, aChild, bChild) {
			return 1
		}
		if stackEntryVisibleNamedUnaryWrapperContains(p, arena, bChild, aChild) {
			return -1
		}
		return 0
	}
	return 0
}

func stackEntryVisibleNamedUnaryWrapperContains(p *Parser, arena *nodeArena, wrapper, direct stackEntry) bool {
	if p == nil || p.language == nil || !stackEntryHasNode(wrapper) || !stackEntryHasNode(direct) {
		return false
	}
	sym := stackEntryNodeSymbol(wrapper)
	if int(sym) >= len(p.language.SymbolMetadata) {
		return false
	}
	meta := p.language.SymbolMetadata[sym]
	if (!meta.Visible && !meta.Named) ||
		stackEntryNodeIsExtra(wrapper) ||
		stackEntryNodeIsMissing(wrapper) ||
		stackEntryNodeHasError(wrapper) ||
		stackEntryNodeChildCount(wrapper) != 1 ||
		stackEntryNodeStartByte(wrapper) != stackEntryNodeStartByte(direct) ||
		stackEntryNodeEndByte(wrapper) != stackEntryNodeEndByte(direct) {
		return false
	}
	child, ok := stackEntryAliasChild(wrapper, arena, 0)
	return ok && stackEntriesHaveSameTreeOrderEnvelope(child, direct)
}

func stackEntryUnaryWrapperContains(p *Parser, arena *nodeArena, wrapper, direct stackEntry, depth int) bool {
	for depth <= maxTreeWalkDepth {
		if stackEntriesHaveSameTreeOrderEnvelope(wrapper, direct) {
			return false
		}
		if !stackEntrySelectionUnaryWrapper(p, arena, wrapper, direct) {
			return false
		}
		child, ok := stackEntryAliasChild(wrapper, arena, 0)
		if !ok {
			return false
		}
		if stackEntriesHaveSameTreeOrderEnvelope(child, direct) {
			return true
		}
		wrapper = child
		depth++
	}
	return false
}

func stackEntrySelectionUnaryWrapper(p *Parser, arena *nodeArena, wrapper, direct stackEntry) bool {
	if p == nil || p.language == nil || !stackEntryHasNode(wrapper) || !stackEntryHasNode(direct) {
		return false
	}
	if !stackEntryTreeOrderTransparentWrapper(p, arena, wrapper) {
		return false
	}
	if stackEntryNodeStartByte(wrapper) != stackEntryNodeStartByte(direct) ||
		stackEntryNodeEndByte(wrapper) != stackEntryNodeEndByte(direct) ||
		stackEntryNodeChildCount(wrapper) != 1 {
		return false
	}
	child, ok := stackEntryAliasChild(wrapper, arena, 0)
	if !ok || !stackEntryHasNode(child) {
		return false
	}
	return stackEntryNodeStartByte(child) == stackEntryNodeStartByte(wrapper) &&
		stackEntryNodeEndByte(child) == stackEntryNodeEndByte(wrapper) &&
		!stackEntryNodeIsExtra(child) &&
		!stackEntryNodeIsMissing(child) &&
		!stackEntryNodeHasError(child)
}

func compareStackEntrySharedPrefixSplice(p *Parser, arena *nodeArena, aParent, bParent stackEntry, childIndex int, depth int) int {
	if depth > maxTreeWalkDepth {
		return 0
	}
	aChild, aOK := stackEntryAliasChild(aParent, arena, childIndex)
	bChild, bOK := stackEntryAliasChild(bParent, arena, childIndex)
	if !aOK || !bOK {
		return 0
	}
	if stackEntryMatchesSharedPrefixSplice(p, arena, aChild, bParent, childIndex, depth+1) {
		return -1
	}
	if stackEntryMatchesSharedPrefixSplice(p, arena, bChild, aParent, childIndex, depth+1) {
		return 1
	}
	return 0
}

func stackEntryMatchesSharedPrefixSplice(p *Parser, arena *nodeArena, wrapper stackEntry, splicedParent stackEntry, startIndex int, depth int) bool {
	const minSharedPrefixChildren = 2
	prefixWrapper, ok := stackEntryTreeOrderPrefixWrapper(p, arena, wrapper)
	if !ok {
		return false
	}
	wrapperChildren := stackEntryNodeChildCount(prefixWrapper)
	splicedChildren := stackEntryNodeChildCount(splicedParent) - startIndex
	if wrapperChildren < minSharedPrefixChildren+1 || splicedChildren < minSharedPrefixChildren+1 {
		return false
	}
	shared := 0
	for shared < wrapperChildren && startIndex+shared < stackEntryNodeChildCount(splicedParent) {
		wrapperChild, wrapperOK := stackEntryAliasChild(prefixWrapper, arena, shared)
		splicedChild, splicedOK := stackEntryAliasChild(splicedParent, arena, startIndex+shared)
		if !wrapperOK || !splicedOK || !stackEntriesHaveSameTreeOrderEnvelope(wrapperChild, splicedChild) {
			break
		}
		shared++
	}
	if shared < minSharedPrefixChildren || shared >= wrapperChildren || startIndex+shared >= stackEntryNodeChildCount(splicedParent) {
		return false
	}
	wrapperNext, wrapperOK := stackEntryAliasChild(prefixWrapper, arena, shared)
	splicedNext, splicedOK := stackEntryAliasChild(splicedParent, arena, startIndex+shared)
	if !wrapperOK || !splicedOK {
		return false
	}
	if stackEntryContainsVisibleNamedStructuralContainer(p, arena, wrapperNext, depth+1) {
		return false
	}
	return stackEntryNodeStartByte(wrapperNext) == stackEntryNodeStartByte(splicedNext) &&
		stackEntryNodeEndByte(splicedNext) == stackEntryNodeEndByte(wrapper)
}

func stackEntryContainsVisibleNamedStructuralContainer(p *Parser, arena *nodeArena, entry stackEntry, depth int) bool {
	if p == nil || p.language == nil || depth > maxTreeWalkDepth || !stackEntryHasNode(entry) {
		return false
	}
	sym := stackEntryNodeSymbol(entry)
	if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
		meta := p.language.SymbolMetadata[idx]
		if (meta.Visible || meta.Named) && stackEntryNodeChildCount(entry) > 0 {
			return true
		}
	}
	if !stackEntryTreeOrderTransparentWrapper(p, arena, entry) {
		return false
	}
	childCount := stackEntryNodeChildCount(entry)
	for i := 0; i < childCount; i++ {
		child, ok := stackEntryAliasChild(entry, arena, i)
		if !ok {
			return false
		}
		if stackEntryContainsVisibleNamedStructuralContainer(p, arena, child, depth+1) {
			return true
		}
	}
	return false
}

func stackEntryTreeOrderPrefixWrapper(p *Parser, arena *nodeArena, entry stackEntry) (stackEntry, bool) {
	for depth := 0; depth < maxTreeWalkDepth; depth++ {
		if !stackEntryTreeOrderTransparentWrapper(p, arena, entry) {
			return stackEntry{}, false
		}
		if stackEntryNodeChildCount(entry) == 0 {
			return entry, true
		}
		child, ok := stackEntryAliasChild(entry, arena, 0)
		if !ok ||
			stackEntryNodeSymbol(child) != stackEntryNodeSymbol(entry) ||
			stackEntryNodeStartByte(child) != stackEntryNodeStartByte(entry) {
			return entry, true
		}
		entry = child
	}
	return stackEntry{}, false
}

func stackEntryTreeOrderTransparentWrapper(p *Parser, arena *nodeArena, entry stackEntry) bool {
	if p == nil || p.language == nil || stackEntryNodeIsExtra(entry) || stackEntryNodeIsMissing(entry) || stackEntryNodeHasError(entry) {
		return false
	}
	sym := stackEntryNodeSymbol(entry)
	if int(sym) >= len(p.language.SymbolMetadata) {
		return false
	}
	meta := p.language.SymbolMetadata[sym]
	if meta.Visible || meta.Named {
		return false
	}
	if stackEntryTreeHasFieldIDs(entry, arena) {
		return false
	}
	productionID := stackEntryNodeProductionID(entry)
	childCount := stackEntryNodeChildCount(entry)
	if fieldMapHasEffectiveFields(p.language, childCount, productionID) {
		return false
	}
	return !languageProductionHasAliasSequence(p.language, productionID, childCount)
}

func languageProductionHasAliasSequence(lang *Language, productionID uint16, childCount int) bool {
	if lang == nil || len(lang.AliasSequences) == 0 {
		return false
	}
	pid := int(productionID)
	if pid < 0 || pid >= len(lang.AliasSequences) {
		return false
	}
	seq := lang.AliasSequences[pid]
	if childCount > len(seq) {
		childCount = len(seq)
	}
	for i := 0; i < childCount; i++ {
		if seq[i] != 0 {
			return true
		}
	}
	return false
}

func stackResultErrorRank(s *glrStack, arena *nodeArena) int {
	if s == nil {
		return 2
	}
	rank := 0
	if len(s.entries) > 0 {
		for i := range s.entries {
			stackEntryResultErrorRank(s.entries[i], arena, &rank)
			if rank == 2 {
				break
			}
		}
		return rank
	}
	for n := s.gss.head; n != nil; n = n.prev {
		stackEntryResultErrorRank(n.entry, arena, &rank)
		if rank == 2 {
			break
		}
	}
	return rank
}

func stackResultDynamicPrecedence(s *glrStack) int32 {
	if s == nil {
		return 0
	}
	var dyn int32
	if len(s.entries) > 0 {
		for i := range s.entries {
			if stackEntryMaterializesForResult(s.entries[i]) {
				dyn += stackEntryDynamicPrecedence(s.entries[i])
			}
		}
		return dyn
	}
	for n := s.gss.head; n != nil; n = n.prev {
		if stackEntryMaterializesForResult(n.entry) {
			dyn += stackEntryDynamicPrecedence(n.entry)
		}
	}
	return dyn
}

func stackEntryResultErrorRank(entry stackEntry, arena *nodeArena, rank *int) {
	if rank == nil || *rank == 2 || !stackEntryMaterializesForResult(entry) {
		return
	}
	if r := cachedStackEntryErrorRank(entry, arena); r > *rank {
		*rank = r
	}
}

// cachedStackEntryErrorRank returns entry's own error-rank contribution (0, 1,
// or 2), independent of any outer accumulator: it is a pure function of
// entry's subtree, computing exactly what computeStackEntryErrorRank always
// computed for entry in isolation. (The shared *rank accumulator threaded
// through stackEntryResultErrorRank/stackResultErrorRank only takes the max
// across sibling top-level entries and early-exits their outer loop once 2 is
// reached — it never changes what a GIVEN entry's own subtree contributes, so
// caching that per-entry contribution here cannot change any caller's final
// result.)
//
// Node and pendingParent entries are the only kinds that can have children
// (stackEntryNodeChildCount is 0 for every other kind), so they are the only
// ones memoized. Nodes use a spare inline byte; pending parents use an arena
// map. A cache miss always falls back to computeStackEntryErrorRank, the same
// recursive walk that ran unconditionally before this cache existed. This is
// what keeps forest
// link-cap eviction scoring (glr_forest.go forestCapReplacementIndex) from
// re-walking whole shared-prefix subtrees per comparison on shapes with many
// raw-distinct alternatives (e.g. C# designer-style repeated-statement
// blocks): once a current-arena (sub)tree's rank is known, later comparisons
// reuse it in O(1). Foreign nodes stay read-only and recompute. See
// Node.errorRankCache and pendingParentErrorRankMemo's doc comment (arena.go)
// for their respective invalidation guarantees.
func cachedStackEntryErrorRank(entry stackEntry, arena *nodeArena) int {
	if n := stackEntryNode(entry); n != nil {
		// Reused nodes belong to an older tree arena and may be read by more
		// than one incremental parser. Keep their immutable read path free of
		// cache writes, and never trust parse-time cache state after result
		// normalization may have rewritten the returned tree.
		cacheInline := arena != nil && n.ownerArena == arena
		if cacheInline && n.errorRankCache != 0 {
			return int(n.errorRankCache - 1)
		}
		rank := computeStackEntryErrorRank(entry, arena)
		if cacheInline {
			n.errorRankCache = uint8(rank + 1)
		}
		return rank
	}
	if pp := stackEntryPendingParent(entry); pp != nil {
		if arena != nil {
			if r, ok := arena.pendingParentErrorRankMemo[pp]; ok {
				return int(r)
			}
		}
		rank := computeStackEntryErrorRank(entry, arena)
		if arena != nil {
			if arena.pendingParentErrorRankMemo == nil {
				arena.pendingParentErrorRankMemo = make(map[*pendingParent]int8, 256)
			}
			arena.pendingParentErrorRankMemo[pp] = int8(rank)
		}
		return rank
	}
	// Every other kind (noTreeNode/compactFullLeaf/nil) has no children (see
	// stackEntryNodeChildCount), so computing its rank is already O(1);
	// caching would cost more than it saves.
	return computeStackEntryErrorRank(entry, arena)
}

// computeStackEntryErrorRank is the recursive computation
// cachedStackEntryErrorRank memoizes: unchanged in substance from the
// original uncached stackEntryResultErrorRank body, so every cache miss (and
// every non-Node compact leaf, which is never cached) still runs this logic.
func computeStackEntryErrorRank(entry stackEntry, arena *nodeArena) int {
	if !stackEntryMaterializesForResult(entry) {
		return 0
	}
	if stackEntryNodeSymbol(entry) == errorSymbol {
		return 2
	}
	rank := 0
	if stackEntryNodeHasError(entry) {
		rank = 1
	}
	for i := 0; i < stackEntryNodeChildCount(entry); i++ {
		child, ok := stackEntryAliasChild(entry, arena, i)
		if !ok {
			continue
		}
		if r := cachedStackEntryErrorRank(child, arena); r > rank {
			rank = r
		}
		if rank == 2 {
			break
		}
	}
	return rank
}

func compareAcceptedStackAliasPreference(p *Parser, arena *nodeArena, a, b *glrStack) int {
	if p == nil || p.language == nil {
		return 0
	}
	if len(p.aliasTargetSymbol) == 0 {
		return 0
	}
	if len(a.entries) > 0 && len(b.entries) > 0 {
		return compareStackEntryAliasPreferenceSlices(p, arena, a.entries, b.entries)
	}
	aCount := stackMaterializingResultEntryCount(a)
	if aCount == 0 || aCount != stackMaterializingResultEntryCount(b) {
		return 0
	}
	const maxBufferedAliasPreferenceEntries = 8
	if aCount > maxBufferedAliasPreferenceEntries {
		if !stackHasCompactResultPayload(a) && !stackHasCompactResultPayload(b) {
			return compareAcceptedStackNodeAliasPreference(p, arena, a, b)
		}
		return 0
	}
	var aBuf, bBuf [maxBufferedAliasPreferenceEntries]stackEntry
	aEntries, aOK := stackMaterializingResultEntries(a, aBuf[:0], aCount)
	bEntries, bOK := stackMaterializingResultEntries(b, bBuf[:0], aCount)
	if !aOK || !bOK {
		return 0
	}
	for i := 0; i < aCount; i++ {
		if cmp := compareStackEntryAliasPreference(p, arena, aEntries[i], bEntries[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareAcceptedStackNodeAliasPreference(p *Parser, arena *nodeArena, a, b *glrStack) int {
	aNodes := resultNodesFromStack(a)
	bNodes := resultNodesFromStack(b)
	if len(aNodes) != len(bNodes) {
		return 0
	}
	for i := range aNodes {
		if cmp := compareNodeAliasPreference(p, arena, aNodes[i], bNodes[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareStackEntryAliasPreferenceSlices(p *Parser, arena *nodeArena, a, b []stackEntry) int {
	aCount := countMaterializingResultEntries(a)
	if aCount == 0 || aCount != countMaterializingResultEntries(b) {
		return 0
	}
	ai, bi := 0, 0
	for compared := 0; compared < aCount; compared++ {
		var aEntry, bEntry stackEntry
		var ok bool
		aEntry, ai, ok = nextMaterializingResultEntry(a, ai)
		if !ok {
			return 0
		}
		bEntry, bi, ok = nextMaterializingResultEntry(b, bi)
		if !ok {
			return 0
		}
		if cmp := compareStackEntryAliasPreference(p, arena, aEntry, bEntry); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func countMaterializingResultEntries(entries []stackEntry) int {
	count := 0
	for i := range entries {
		if stackEntryMaterializesForResult(entries[i]) {
			count++
		}
	}
	return count
}

func nextMaterializingResultEntry(entries []stackEntry, start int) (stackEntry, int, bool) {
	for i := start; i < len(entries); i++ {
		if stackEntryMaterializesForResult(entries[i]) {
			return entries[i], i + 1, true
		}
	}
	return stackEntry{}, len(entries), false
}

func stackEntryMaterializesForResult(entry stackEntry) bool {
	return stackEntryNode(entry) != nil || stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
}

func stackEntryHasCompactResultPayload(entry stackEntry) bool {
	return stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
}

func stackHasCompactResultPayload(s *glrStack) bool {
	if len(s.entries) > 0 {
		for i := range s.entries {
			if stackEntryHasCompactResultPayload(s.entries[i]) {
				return true
			}
		}
		return false
	}
	for n := s.gss.head; n != nil; n = n.prev {
		if stackEntryHasCompactResultPayload(n.entry) {
			return true
		}
	}
	return false
}

func stackMaterializingResultEntryCount(s *glrStack) int {
	if len(s.entries) > 0 {
		return countMaterializingResultEntries(s.entries)
	}
	if s.gss.head == nil {
		return 0
	}
	count := 0
	for n := s.gss.head; n != nil; n = n.prev {
		if stackEntryMaterializesForResult(n.entry) {
			count++
		}
	}
	return count
}

func stackMaterializingResultEntries(s *glrStack, dst []stackEntry, materializingCount int) ([]stackEntry, bool) {
	if materializingCount == 0 || cap(dst) < materializingCount {
		return nil, false
	}
	dst = dst[:materializingCount]
	if len(s.entries) > 0 {
		index := 0
		for i := range s.entries {
			if !stackEntryMaterializesForResult(s.entries[i]) {
				continue
			}
			if index >= materializingCount {
				return nil, false
			}
			dst[index] = s.entries[i]
			index++
		}
		return dst, index == materializingCount
	}
	index := materializingCount - 1
	for n := s.gss.head; n != nil; n = n.prev {
		if !stackEntryMaterializesForResult(n.entry) {
			continue
		}
		if index < 0 {
			return nil, false
		}
		dst[index] = n.entry
		index--
	}
	return dst, index == -1
}

func resultNodesFromStack(s *glrStack) []*Node {
	if len(s.entries) > 0 {
		count := 0
		for i := range s.entries {
			if stackEntryNode(s.entries[i]) != nil {
				count++
			}
		}
		if count == 0 {
			return nil
		}
		nodes := make([]*Node, 0, count)
		for i := range s.entries {
			if node := stackEntryNode(s.entries[i]); node != nil {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	if s.gss.head == nil {
		return nil
	}
	return nodesFromGSS(s.gss)
}

func compareNodeAliasPreference(p *Parser, arena *nodeArena, a, b *Node) int {
	if a == b || a == nil || b == nil {
		return 0
	}
	aChildCount := nodeChildCountNoMaterialize(a)
	bChildCount := nodeChildCountNoMaterialize(b)
	if a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		a.isExtra() != b.isExtra() ||
		a.isMissing() != b.isMissing() ||
		aChildCount != bChildCount {
		return 0
	}
	if a.symbol != b.symbol {
		aType := a.Type(p.language)
		bType := b.Type(p.language)
		if aType == bType {
			for i := 0; i < aChildCount; i++ {
				aChild, aOK := nodeChildEntryAtNoMaterialize(a, i)
				bChild, bOK := nodeChildEntryAtNoMaterialize(b, i)
				if !aOK || !bOK {
					return 0
				}
				if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
					return cmp
				}
			}
			return 0
		}
		aAlias := p.isAliasTargetSymbol(a.symbol)
		bAlias := p.isAliasTargetSymbol(b.symbol)
		if aAlias != bAlias {
			if aAlias {
				return 1
			}
			return -1
		}
		return 0
	}
	for i := 0; i < aChildCount; i++ {
		aChild, aOK := nodeChildEntryAtNoMaterialize(a, i)
		bChild, bOK := nodeChildEntryAtNoMaterialize(b, i)
		if !aOK || !bOK {
			return 0
		}
		if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareStackEntryAliasPreference(p *Parser, arena *nodeArena, a, b stackEntry) int {
	if a.node == b.node && a.kind == b.kind {
		return 0
	}
	if !stackEntryMaterializesForResult(a) || !stackEntryMaterializesForResult(b) {
		return 0
	}
	if stackEntryNode(a) != nil && stackEntryNode(b) != nil {
		return compareNodeAliasPreference(p, arena, stackEntryNode(a), stackEntryNode(b))
	}
	if stackEntryNodeStartByte(a) != stackEntryNodeStartByte(b) ||
		stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b) ||
		stackEntryNodeIsExtra(a) != stackEntryNodeIsExtra(b) ||
		stackEntryNodeIsMissing(a) != stackEntryNodeIsMissing(b) ||
		stackEntryNodeChildCount(a) != stackEntryNodeChildCount(b) {
		return 0
	}
	if stackEntryNodeSymbol(a) != stackEntryNodeSymbol(b) {
		aType := stackEntryTypeName(p, a)
		bType := stackEntryTypeName(p, b)
		if aType == bType {
			for i := 0; i < stackEntryNodeChildCount(a); i++ {
				aChild, aOK := stackEntryAliasChild(a, arena, i)
				bChild, bOK := stackEntryAliasChild(b, arena, i)
				if !aOK || !bOK {
					return 0
				}
				if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
					return cmp
				}
			}
			return 0
		}
		aAlias := p.isAliasTargetSymbol(stackEntryNodeSymbol(a))
		bAlias := p.isAliasTargetSymbol(stackEntryNodeSymbol(b))
		if aAlias != bAlias {
			if aAlias {
				return 1
			}
			return -1
		}
		return 0
	}
	for i := 0; i < stackEntryNodeChildCount(a); i++ {
		aChild, aOK := stackEntryAliasChild(a, arena, i)
		bChild, bOK := stackEntryAliasChild(b, arena, i)
		if !aOK || !bOK {
			return 0
		}
		if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func stackEntryAliasChild(entry stackEntry, arena *nodeArena, i int) (stackEntry, bool) {
	if n := stackEntryNode(entry); n != nil {
		return nodeChildEntryAtNoMaterialize(n, i)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if i < 0 || i >= parent.childEntryCount() {
			return stackEntry{}, false
		}
		return parent.childEntry(arena, i), true
	}
	return stackEntry{}, false
}

func stackEntryTypeName(p *Parser, entry stackEntry) string {
	if stackEntryNodeSymbol(entry) == errorSymbol {
		return "ERROR"
	}
	if p == nil || p.language == nil {
		return ""
	}
	sym := stackEntryNodeSymbol(entry)
	if int(sym) >= len(p.language.SymbolNames) {
		return ""
	}
	return unescapePunctuationSymbolName(p.language.SymbolNames[sym])
}

func (p *Parser) isAliasTargetSymbol(sym Symbol) bool {
	if p == nil || int(sym) >= len(p.aliasTargetSymbol) {
		return false
	}
	return p.aliasTargetSymbol[sym]
}

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	return p != nil && symbolIsNamed(p.language, sym)
}

func nodesFromGSS(stack gssStack) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if stackEntryNode(n.entry) != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if node := stackEntryNode(n.entry); node != nil {
			nodes[i] = node
			i--
		}
	}
	return nodes
}

func nodesFromGSSMaterializingCompactFullLeaves(p *Parser, stack gssStack, arena *nodeArena) ([]*Node, ParseStopReason) {
	if stack.head == nil {
		return nil, ParseStopNone
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if stackEntryNode(n.entry) != nil || stackEntryCompactFullLeaf(n.entry) != nil || stackEntryPendingParent(n.entry) != nil {
			count++
		}
	}
	if count == 0 {
		return nil, ParseStopNone
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if i&255 == 0 {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				return nil, reason
			}
		}
		if node := materializeStackEntryPayloadWithParser(p, arena, &n.entry, compactFullLeafMaterializeForFinalTree, pendingParentMaterializeForFinalTree); node != nil {
			nodes[i] = node
			i--
		}
	}
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		return nil, reason
	}
	return nodes, ParseStopNone
}

func filterZeroWidthExtras(nodes []*Node, arena *nodeArena) []*Node {
	if len(nodes) == 0 {
		return nodes
	}
	keep := 0
	for _, n := range nodes {
		if n == nil || !n.isExtra() || n.endByte > n.startByte {
			keep++
		}
	}
	if keep == len(nodes) || keep == 0 {
		return nodes
	}
	filtered := make([]*Node, 0, keep)
	for _, n := range nodes {
		if n != nil && n.isExtra() && n.endByte == n.startByte {
			continue
		}
		filtered = append(filtered, n)
	}
	if arena != nil {
		out := arena.allocNodeSlice(len(filtered))
		copy(out, filtered)
		return out
	}
	return filtered
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		tree := parseErrorTreeWithArena(source, p.language, arena)
		tree.setParseStopReason(reason)
		return tree
	}
	var nodes []*Node
	for i := range stack {
		if i&255 == 0 {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				tree := parseErrorTreeWithArena(source, p.language, arena)
				tree.setParseStopReason(reason)
				return tree
			}
		}
		if node := materializeStackEntryPayloadWithParser(p, arena, &stack[i], compactFullLeafMaterializeForFinalTree, pendingParentMaterializeForFinalTree); node != nil {
			nodes = append(nodes, node)
		}
	}
	return p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
}

func (p *Parser) buildResultFromNodes(nodes []*Node, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		tree := parseErrorTreeWithArena(source, p.language, arena)
		tree.setParseStopReason(reason)
		return tree
	}
	if len(nodes) == 0 {
		if isWhitespaceOnlySource(source) {
			return newTreeWithArenas(nil, source, p.language, arena, nil)
		}
		return parseErrorTreeWithArena(source, p.language, arena)
	}

	builder := newResultRootBuild(p, source, arena, oldTree, reuseState, linkScratch)
	nodes = builder.prepareRootNodes(nodes)

	if len(nodes) == 1 {
		return builder.buildSingleRootTree(nodes[0])
	}

	if tree := builder.tryBuildRealRootTree(nodes); tree != nil {
		return tree
	}

	if builder.hasExpectedRoot && len(nodes) > 1 {
		nodes = flattenRootSelfFragments(nodes, arena, builder.expectedRootSymbol)
	}
	return builder.buildSyntheticRootTree(nodes)
}

// maxTreeWalkDepth prevents stack overflow in recursive tree walkers when
// parsing with grammargen-produced grammars that can create pathologically deep
// hidden-node chains (e.g. Scala with >1M levels).
const maxTreeWalkDepth = 5000
