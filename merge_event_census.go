//go:build gts_merge_census

package gotreesitter

import "sync"

// Merge-event census, production side (stage M0 of
// spec.merge-time-election.v1).
//
// The lane's thesis (finding.late-election-unifying-cause) is that the
// reference runtime collapses duplicate stack versions AT THE MERGE and
// carries one version forward, while this parser refuses the merge when the
// subtree shapes differ, carries both versions to EOF, and elects at the end.
// Nobody has taken the direct measurement: how many merges does the reference
// runtime perform, how many does this parser perform, and which gate refuses
// the rest. This file publishes the production half of that measurement.
//
// The whole file sits behind the gts_merge_census build tag. The shipped
// build compiles merge_event_census_disabled.go instead, where
// mergeCensusEnabled is a false constant and every hook has an empty body. A
// constant false guard removes the guarded blocks at compile time, so the
// shipped binary keeps the exact instruction stream it had before the census
// existed. merge_event_census_inert_test.go is the ratchet for that claim.
//
// The census NEVER influences a merge decision. Every hook reads the same
// values the gate already read and returns nothing.

// mergeCensusEnabled reports whether the census is compiled in. It is a
// constant so every guarded block is removed from the default build.
const mergeCensusEnabled = true

// MergeEventCensusCounts is one parse's production merge accounting.
//
// The refusal fields answer stage M1's question directly: which gate must go
// first, and what does removing it cost. Every field name states the gate it
// counts, not a mechanism the gate is believed to protect.
type MergeEventCensusCounts struct {
	// Attempts is the number of times a production merge front door was
	// entered with a candidate pair. The three front doors are
	// tryGSSMainMergeForParser, tryGSSMainMergeForParserPhase and
	// tryGSSMainMergeResult. The reference runtime's equivalent is a call to
	// ts_stack_merge (merge_attempts_proxy).
	Attempts uint64
	// Successes is the number of attempts that collapsed two versions into
	// one. The reference runtime's equivalent is a ts_stack_merge call that
	// returned true (merge_successes_proxy). Successes divided by the
	// reference runtime's successes is the lane's progress ratio.
	Successes uint64
	// MixedRepresentationMergeAttempts and MixedRepresentationMergeSuccesses
	// count certified flat/GSS representation joins separately. These joins
	// remove duplicate Go stack representations; the C runtime has no
	// corresponding version merge, so they do not enter Successes.
	MixedRepresentationMergeAttempts  uint64
	MixedRepresentationMergeSuccesses uint64

	// RefuseNoGSSHead counts pairs where at least one side carried no packed
	// head. A glrStack packs lazily (ensureGSS, glr.go:466-471), so a nil head
	// means the candidate is still in flat-slice form and was never presented
	// to the graph at all. The reference runtime has no equivalent state: every
	// version is a stack-node chain from creation, so this whole class is a
	// refusal with no counterpart. RefuseNoGSSHeadBoth and RefuseNoGSSHeadOne
	// split it, because "neither side was packed" and "one side was packed"
	// need different work in stage M1. A certified mixed-representation join
	// increments the one-flat counter as a baseline gate observation, while its
	// separate fields record the successful representation join.
	RefuseNoGSSHead     uint64
	RefuseNoGSSHeadBoth uint64
	RefuseNoGSSHeadOne  uint64
	// RefuseStatus counts pairs rejected because one side was dead, or
	// because the accepted flags differed.
	RefuseStatus uint64
	// RefuseScoreOrShifted counts the score-equality gate (glr.go:3311) plus
	// its preflight fast reject in tryGSSMainMergeResult. The reference
	// runtime merges across dynamic-precedence differences and elects per
	// link (stack.c:216-218), so this whole class is a refusal the reference
	// runtime does not make. Stage M1 item 1.
	RefuseScoreOrShifted uint64
	// RefuseScoreOnly and RefuseShiftedOnly split the gate above so the
	// worker who deletes it can attribute each half. RefuseShiftedOnly is the
	// part with no reference-runtime counterpart at all: C has no shifted
	// field.
	RefuseScoreOnly   uint64
	RefuseShiftedOnly uint64
	// RefuseStateOrOffset counts the Tier-1 key mismatch: different top state
	// or different byte offset. The reference runtime refuses these too
	// (stack.c:709-719), so this class is agreement, not divergence.
	RefuseStateOrOffset uint64
	// RefuseCleanZero counts the clean-zero-error merge gate
	// (glr.go:3317-3318). Stage M2, not M1.
	RefuseCleanZero uint64
	// RefuseErrorCost counts the recovery-cost rejection in
	// tryGSSMainMergeResult. The reference runtime keys error cost into the
	// merge test as well (stack.c:713), so this class is agreement.
	RefuseErrorCost uint64
	// RefuseDistinctShapes counts the distinct-materializing-shapes refusal
	// (glr.go:4401-4407). This is the refusal the finding names: where the
	// reference runtime packs the differing subtree as an extra link
	// (stack.c:249-263), production declines the merge and carries both
	// versions. Stage M1 item 2.
	RefuseDistinctShapes uint64
	// RefuseMergeFailed counts attempts that passed every gate and still did
	// not collapse, because the graph mutation itself declined.
	RefuseMergeFailed uint64

	// LinkPayloadTests is the number of Tier-2 link-payload comparisons
	// (stackEntryPayloadsEquivalentIgnoringDynamicWithScratch), the port of
	// stack__subtree_is_equivalent (stack.c:181-198).
	LinkPayloadTests uint64
	// LinkPayloadDeepAccepts and LinkPayloadDeepRefusals split those tests by
	// production's verdict.
	LinkPayloadDeepAccepts  uint64
	LinkPayloadDeepRefusals uint64
	// LinkPayloadShallowWouldAccept is the actionable number: comparisons
	// where the reference runtime's SHALLOW test accepts and production's
	// DEEP test refuses. Every one of these turns a reference-runtime Tier-2
	// collapse into a production append. Stage M1 item 3 removes exactly this
	// class.
	LinkPayloadShallowWouldAccept uint64
	// LinkPayloadDeepWouldAccept is the opposite direction: production
	// accepts where the shallow test refuses. A non-zero count here means
	// adopting the shallow test alone would LOSE collapses, so it must be
	// reported rather than assumed empty.
	LinkPayloadDeepWouldAccept uint64
	// LinkPayloadPending counts comparisons routed to the pending-parent
	// branch, where no shallow comparison is defined because the payload has
	// no materialized node yet. Reported, never folded into the two classes
	// above.
	LinkPayloadPending uint64

	// CompactLinkUnionAttempts and the four outcome fields mirror the compact
	// core's own always-on link-union counters (internal/parsercorephase0),
	// snapshotted at a compact acceptance. They are recorded only when the
	// compact route reaches an accept.
	CompactLinkUnionAttempts           uint64
	CompactLinkUnionDuplicateNoop      uint64
	CompactLinkUnionPrecedenceReplaced uint64
	CompactLinkUnionRecursiveChanged   uint64
	CompactLinkUnionAlternateAppended  uint64
	CompactLinkUnionRejected           uint64
	CompactPhysicalHeadMergeAttempts   uint64
	CompactPhysicalHeadMergeSuccesses  uint64
	CompactPhysicalHeadMergeInputLinks uint64
	CompactAcceptancesObserved         uint64
}

var mergeCensusState struct {
	mu     sync.Mutex
	counts MergeEventCensusCounts
}

// MergeEventCensusBuilt reports whether this binary carries the merge-event
// census. Only a gts_merge_census build does.
func MergeEventCensusBuilt() bool { return true }

// MergeEventCensusReset clears the accumulated counts. Call it before the
// parse under measurement.
func MergeEventCensusReset() {
	mergeCensusState.mu.Lock()
	mergeCensusState.counts = MergeEventCensusCounts{}
	mergeCensusState.mu.Unlock()
}

// MergeEventCensusSnapshot returns the counts accumulated since the last
// reset.
func MergeEventCensusSnapshot() MergeEventCensusCounts {
	mergeCensusState.mu.Lock()
	out := mergeCensusState.counts
	mergeCensusState.mu.Unlock()
	return out
}

func mergeCensusAdd(field *uint64, delta uint64) {
	mergeCensusState.mu.Lock()
	*field += delta
	mergeCensusState.mu.Unlock()
}

func mergeCensusRecordAttempt() { mergeCensusAdd(&mergeCensusState.counts.Attempts, 1) }

func mergeCensusRecordSuccess() { mergeCensusAdd(&mergeCensusState.counts.Successes, 1) }

func mergeCensusRecordMixedRepresentationAttempt() {
	mergeCensusState.mu.Lock()
	mergeCensusState.counts.MixedRepresentationMergeAttempts++
	mergeCensusState.counts.RefuseNoGSSHead++
	mergeCensusState.counts.RefuseNoGSSHeadOne++
	mergeCensusState.mu.Unlock()
}

func mergeCensusRecordMixedRepresentationSuccess() {
	mergeCensusAdd(&mergeCensusState.counts.MixedRepresentationMergeSuccesses, 1)
}

func mergeCensusRecordMergeFailed() { mergeCensusAdd(&mergeCensusState.counts.RefuseMergeFailed, 1) }

func mergeCensusRecordDistinctShapes() {
	mergeCensusAdd(&mergeCensusState.counts.RefuseDistinctShapes, 1)
}

func mergeCensusRecordErrorCost() { mergeCensusAdd(&mergeCensusState.counts.RefuseErrorCost, 1) }

// mergeCensusRecordScorePreflight attributes the fast score reject in
// tryGSSMainMergeResult (glr.go:4381-4386) to the same class as the gate it
// front-runs, so the two sites never double-count differently.
func mergeCensusRecordScorePreflight() {
	mergeCensusState.mu.Lock()
	mergeCensusState.counts.RefuseScoreOrShifted++
	mergeCensusState.counts.RefuseScoreOnly++
	mergeCensusState.mu.Unlock()
}

// mergeCensusAttributeForParserRefusal attributes one refusal by
// gssMainCanMergeForParser, whose first gate is the recovery-cost comparison
// and whose second is gssMainCanMergeWithScratch.
func mergeCensusAttributeForParserRefusal(p *Parser, a, b *glrStack) {
	if cRecoveryMergeCostsDifferForParser(p, a, b) {
		mergeCensusRecordErrorCost()
		return
	}
	var scratch *glrMergeScratch
	if p != nil {
		scratch = p.mergeScratch
	}
	mergeCensusRecordGateRefusal(scratch, a, b)
}

// mergeCensusRecordGateRefusal attributes one refusal by
// gssMainCanMergeWithScratch. It re-evaluates the gate's own conditions in
// the gate's own order. The gate is pure, so the attribution is exact rather
// than inferred.
func mergeCensusRecordGateRefusal(scratch *glrMergeScratch, a, b *glrStack) {
	mergeCensusState.mu.Lock()
	defer mergeCensusState.mu.Unlock()
	counts := &mergeCensusState.counts
	switch {
	case a == nil || b == nil:
		counts.RefuseStatus++
	case a.gss.head == nil || b.gss.head == nil:
		counts.RefuseNoGSSHead++
		if a.gss.head == nil && b.gss.head == nil {
			counts.RefuseNoGSSHeadBoth++
		} else {
			counts.RefuseNoGSSHeadOne++
		}
	case a.dead || b.dead || a.accepted != b.accepted:
		counts.RefuseStatus++
	case a.score != b.score || a.shifted != b.shifted:
		counts.RefuseScoreOrShifted++
		if a.score != b.score {
			counts.RefuseScoreOnly++
		} else {
			counts.RefuseShiftedOnly++
		}
	case a.top().state != b.top().state || a.byteOffset != b.byteOffset:
		counts.RefuseStateOrOffset++
	default:
		counts.RefuseCleanZero++
	}
	_ = scratch
}

// mergeCensusRecordLinkPayload records one Tier-2 link-payload comparison and
// the divergence between production's deep verdict and the reference
// runtime's shallow verdict.
func mergeCensusRecordLinkPayload(a, b stackEntry, deep bool) {
	pending := stackEntryPendingParent(a) != nil || stackEntryPendingParent(b) != nil
	shallow := false
	if !pending {
		shallow = mergeCensusShallowEquivalent(a, b)
	}
	mergeCensusState.mu.Lock()
	counts := &mergeCensusState.counts
	counts.LinkPayloadTests++
	if deep {
		counts.LinkPayloadDeepAccepts++
	} else {
		counts.LinkPayloadDeepRefusals++
	}
	switch {
	case pending:
		counts.LinkPayloadPending++
	case shallow && !deep:
		counts.LinkPayloadShallowWouldAccept++
	case deep && !shallow:
		counts.LinkPayloadDeepWouldAccept++
	}
	mergeCensusState.mu.Unlock()
}

// mergeCensusShallowEquivalent is the port of the reference runtime's
// stack__subtree_is_equivalent (stack.c:181-198): equal symbol, a
// both-have-errors shortcut, then equal padding, size, child count and extra
// flag.
//
// Two components of the reference test are deliberately absent, and their
// absence is stated rather than hidden:
//
//   - External scanner state. Production's scanner is a parser singleton
//     (glr.go:1978-1982), so the component is tautological today. Stage M2
//     gives it a per-stack identity before it joins any key.
//   - Padding as a separate length. The reference runtime stores padding and
//     size; production stores start and end bytes. Equal start plus equal end
//     is equal padding plus equal size when the two payloads follow the same
//     predecessor, which is the only position this test is applied from. The
//     spec records the exact mapping as open question 4.
func mergeCensusShallowEquivalent(a, b stackEntry) bool {
	if stackEntryNodeSymbol(a) != stackEntryNodeSymbol(b) {
		return false
	}
	if stackEntryNodeHasError(a) && stackEntryNodeHasError(b) {
		return true
	}
	return stackEntryNodeStartByte(a) == stackEntryNodeStartByte(b) &&
		stackEntryNodeEndByte(a) == stackEntryNodeEndByte(b) &&
		stackEntryNodeChildCount(a) == stackEntryNodeChildCount(b) &&
		stackEntryNodeIsExtra(a) == stackEntryNodeIsExtra(b)
}

// mergeCensusRecordCompactLinkUnion snapshots the compact core's own
// always-on link-union counters at a compact acceptance. The compact core
// keeps cumulative per-parse totals, so the census stores the last observed
// values rather than summing them.
func mergeCensusRecordCompactLinkUnion(
	attempts, duplicate, precedence, recursive, alternate, rejected uint64,
	physicalAttempts, physicalSuccesses, physicalInputLinks uint64,
) {
	mergeCensusState.mu.Lock()
	counts := &mergeCensusState.counts
	counts.CompactAcceptancesObserved++
	counts.CompactLinkUnionAttempts = attempts
	counts.CompactLinkUnionDuplicateNoop = duplicate
	counts.CompactLinkUnionPrecedenceReplaced = precedence
	counts.CompactLinkUnionRecursiveChanged = recursive
	counts.CompactLinkUnionAlternateAppended = alternate
	counts.CompactLinkUnionRejected = rejected
	counts.CompactPhysicalHeadMergeAttempts = physicalAttempts
	counts.CompactPhysicalHeadMergeSuccesses = physicalSuccesses
	counts.CompactPhysicalHeadMergeInputLinks = physicalInputLinks
	mergeCensusState.mu.Unlock()
}
