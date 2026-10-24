//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
	"math"
)

// This adapter only binds the core loop to scheduler polling and provenance.
// The core owns every stack transition and its publication.
type singleVersionAdapter struct {
	scheduler                                  *diagnosticParserCoreGenericScheduler
	dependencyBefore                           uint32
	dependencyActive                           bool
	leafBefore                                 uint32
	firstRow                                   uint32
	rowPending, pollPending, dependencyPending bool
}

func (s *diagnosticParserCoreGenericScheduler) singleVersionEligible() bool {
	if s.freshSessionOwner == nil || s.fullReceipts() || !s.corridorCanLoop() ||
		!s.singleHeaderProbeIsIdentity() || s.token.Missing || s.token.NoLookahead ||
		(s.options.eagerMaterializer != nil && s.options.eagerMaterializer.eager) {
		return false
	}
	h := &s.headers[0]
	if h.isRecoveryLineage() || h.recoveryRegion() != nil || h.isRecoveryCosted() ||
		h.creationSeq >= math.MaxUint32 || !h.dropCohortRefs.Empty() ||
		h.dropCohortRefs.Overflowed() || h.dropCohortRefs.Blended() {
		return false
	}
	// Contextual close-angle deferral depends on each landing state. Let the
	// existing corridor perform those checks, before spending classification.
	return s.tokenSource == nil || s.tokenSource.lexer == nil ||
		!tokenMaybeContextualCloseAngle(s.tokenSource.language, &s.token)
}

func (a *singleVersionAdapter) SingleVersionRow(state core.StateID, symbol core.Symbol) (core.ActionRow, bool) {
	s := a.scheduler
	if a.rowPending {
		a.rowPending = false
		return s.corridorRows[a.firstRow], true
	}
	if int(state) >= len(s.corridor.stateBlockOffset) {
		return core.ActionRow{}, false
	}
	prog := s.corridor.prog
	body := corridorDispatch(prog, s.corridor.stateBlockOffset[state], Symbol(symbol))
	if body == 0 {
		return core.ActionRow{}, false
	}
	op := parserCoreCorridorOpcode(prog[body] & corridorOpcodeMask)
	if !s.corridorTokenGateOpen(op) {
		return core.ActionRow{}, false
	}
	var index uint32
	switch op {
	case corridorOpShift, corridorOpReduceChain, corridorOpReduceShift:
		index = prog[body+1]
	case corridorOpReduce:
		index = prog[body+2]
	default:
		return core.ActionRow{}, false
	}
	return s.corridorRows[index], true
}

func (a *singleVersionAdapter) BeforeSingleVersion(kind core.ActionType) error {
	s := a.scheduler
	if a.pollPending {
		a.pollPending = false
	} else if err := s.pollStopControl(); err != nil {
		return err
	}
	if s.dispatches >= s.options.MaxDispatches {
		// Preserve classification before the ordinary dispatch-cap failure.
		s.corridorRecordClassification(1)
		return s.reserveDispatches(1)
	}
	a.leafBefore = uint32(s.compact.SubtreeCount())
	// Every admitted transition preserves the sole predecessor path count.
	// The elected token and its examined frontier stay fixed through this batch.
	// Keep per-output publication (including reused payload IDs), but establish
	// the forward-dependency proof only once for that unchanged election.
	if a.dependencyPending {
		a.dependencyBefore, a.dependencyActive = s.beginCompactReuseDependency(s.token)
		a.dependencyPending = false
	}
	return nil
}

func (a *singleVersionAdapter) AfterSingleVersion(head core.Head, kind core.ActionType, shifted bool) (err error) {
	s := a.scheduler
	h := &s.headers[0]
	s.corridorRecordClassification(1)
	s.dispatches++
	h.head = head
	h.shifted = shifted
	if kind == core.ActionReduce {
		h.paused = false
		h.clearZeroWidthReopened()
		s.invalidateVerifierHeaderBinding()
		s.work.Reductions++
	} else {
		if s.token.ExternalScannerToken {
			h.clearZeroWidthReopened()
		}
		markDiagnosticParserCoreExternalLineage(h, s.token)
		s.work.OrdinaryShifts++
	}
	if h.convergedReductionSplit {
		h.lastPersistedHead = head
		h.lastPersistedAltSet = h.altSet
		h.lastPersistedBlended = h.blended
	}
	s.epochProgress = true
	s.work.Dispatches++
	s.skipCanonicalProbe()
	s.endCompactReuseDependency(a.dependencyBefore, a.dependencyActive, &err)
	a.dependencyBefore = uint32(s.compact.SubtreeCount())
	a.dependencyActive = a.dependencyActive && !s.reuseDependencies.disabled
	if kind == core.ActionReduce || s.token.ExternalScannerToken {
		s.endCompactLeafReceipt(a.leafBefore, s.token, &err)
	}
	return err
}

func (s *diagnosticParserCoreGenericScheduler) runSingleVersion(firstRow uint32) (core.SingleVersionResult, error) {
	s.singleVersion.scheduler = s
	s.singleVersion.firstRow = firstRow
	s.singleVersion.rowPending, s.singleVersion.pollPending, s.singleVersion.dependencyPending = true, true, true
	defer func() { s.singleVersion.scheduler = nil }()
	h := &s.headers[0]
	publication := core.ShiftHeadLineage{
		Owner: uint32(h.creationSeq) + 1, PreviousHead: h.lastPersistedHead,
		Set: h.altSet, Lineage: h.cleanPathLineage, Rank: h.cleanPathRank,
		Blended: h.blended, PublishLineage: h.convergedReductionSplit,
		SetDirty: h.altSet != h.lastPersistedAltSet || h.blended != h.lastPersistedBlended,
	}
	token := core.Token{Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte,
		EndByte: s.token.EndByte, External: s.token.ExternalScannerToken,
		LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(s.token, s.options.captureLexerSkippedPrefixProvenance)}
	return s.compact.RunSingleVersionOwned(*s.freshSessionOwner, h.head, token, publication, 256, &s.singleVersion, &s.singleVersion)
}
