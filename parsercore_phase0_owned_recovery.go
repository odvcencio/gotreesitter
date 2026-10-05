//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func compactOwnedRecoveryDecline(index int, detail string) *diagnosticParserCoreGenericUnsupported {
	return &diagnosticParserCoreGenericUnsupported{
		boundary: DiagnosticParserCoreRecovery, detail: detail, headerIndex: index,
	}
}

// ownedRecoveryBetterVersionExists compares actual live positions. Paused
// versions cannot suppress a recovery. Finished trees compete independently
// of their former position, as C's finished_tree does.
func (s *diagnosticParserCoreGenericScheduler) ownedRecoveryBetterVersionExists(
	index int,
	current diagnosticParserCoreRecoveryCondenseEntry,
	cost uint32,
	symbols []core.SelectedSymbolPolicy,
	source *diagnosticParserCoreRecoveryCostSource,
	memo *core.RecoveryCostMemo,
) (bool, error) {
	status := current.status
	status.Cost, status.IsInError = cost, false
	for otherIndex := range s.headers {
		if otherIndex == index || s.headers[otherIndex].paused {
			continue
		}
		other, supported, err := s.recoveryCondenseEntry(s.headers[otherIndex], symbols, source, memo)
		if err != nil {
			return false, err
		}
		if !supported {
			return false, errDiagnosticParserCoreLineageCostUnavailable
		}
		if s.headers[otherIndex].accepted {
			if other.status.Cost <= cost {
				return true, nil
			}
			continue
		}
		if other.key.byteOffset < current.key.byteOffset {
			continue
		}
		switch core.RecoveryCompareVersions(status, other.status) {
		case core.RecoveryComparisonTakeRight:
			return true, nil
		case core.RecoveryComparisonPreferRight:
			if current.key.state == other.key.state && current.key.byteOffset == other.key.byteOffset &&
				current.key.cost == other.key.cost && current.key.checkpoint == other.key.checkpoint {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *diagnosticParserCoreGenericScheduler) ownedRecoverySummaryCandidate(
	index int,
	current diagnosticParserCoreRecoveryCondenseEntry,
	symbols []core.SelectedSymbolPolicy,
	source *diagnosticParserCoreRecoveryCostSource,
	memo *core.RecoveryCostMemo,
) (core.StackSummaryCandidate, bool, error) {
	if s.token.Symbol == errorSymbol {
		return core.StackSummaryCandidate{}, false, nil
	}
	candidates, err := s.compact.StackSummaryCandidates(s.headers[index].head, cRecoverMaxSummaryDepth)
	if err != nil {
		return core.StackSummaryCandidate{}, false, err
	}
	position := current.key.byteOffset
	for candidateIndex, candidate := range candidates {
		if candidateIndex&63 == 0 {
			if err := s.pollStopControl(); err != nil {
				return core.StackSummaryCandidate{}, false, err
			}
		}
		if candidate.State() == 0 || candidate.ByteOffset() >= position {
			continue
		}
		wouldMerge := false
		for otherIndex := range s.headers {
			other := &s.headers[otherIndex]
			if other.accepted || other.recoveryRegion() != nil {
				continue
			}
			state, byteOffset, err := s.compact.Boundary(other.head)
			if err != nil {
				return core.StackSummaryCandidate{}, false, err
			}
			if state == candidate.State() && byteOffset == position {
				wouldMerge = true
				break
			}
		}
		if wouldMerge {
			continue
		}
		cost := uint64(current.status.Cost) + uint64(candidate.Depth())*core.RecoveryCostPerSkippedTree +
			uint64(position-candidate.ByteOffset())*core.RecoveryCostPerSkippedChar +
			uint64(source.rowAt(position)-source.rowAt(candidate.ByteOffset()))*core.RecoveryCostPerSkippedLine
		if cost > math.MaxUint32 {
			return core.StackSummaryCandidate{}, false, errors.New("parser-core phase zero: owned summary cost overflow")
		}
		better, err := s.ownedRecoveryBetterVersionExists(index, current, uint32(cost), symbols, source, memo)
		if err != nil {
			return core.StackSummaryCandidate{}, false, err
		}
		if better {
			break
		}
		row, err := s.compact.Actions(candidate.State(), core.Symbol(s.token.Symbol))
		if err != nil {
			return core.StackSummaryCandidate{}, false, err
		}
		if row.Len() == 0 {
			continue
		}
		recoverable, err := s.compact.StackSummaryCandidateRecoverable(candidate)
		if err != nil {
			return core.StackSummaryCandidate{}, false, err
		}
		if !recoverable {
			return core.StackSummaryCandidate{}, false, errDiagnosticParserCoreLineageCostUnavailable
		}
		return candidate, true, nil
	}
	return core.StackSummaryCandidate{}, false, nil
}

// dispatchOwnedRecoveryRegion executes one C recovery advance at EOF.
// Strategy one appends a version before the original accepts its ERROR root.
func (s *diagnosticParserCoreGenericScheduler) dispatchOwnedRecoveryRegion(
	index int,
) (_ *diagnosticParserCoreGenericUnsupported, err error) {
	if s == nil || s.compact == nil || index < 0 || index >= len(s.headers) {
		return nil, errors.New("parser-core phase zero: owned recovery index is invalid")
	}
	original := s.headers[index]
	region := original.recoveryRegion()
	request := s.versionLexerRequestForHeader(index)
	if region == nil || len(region.children) == 0 || request == nil || request.state != 0 {
		return compactOwnedRecoveryDecline(index, "owned recovery requires an authenticated error-state token"), nil
	}
	if s.token != request.token || s.token.StartByte < region.endByte ||
		s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
		return compactOwnedRecoveryDecline(index, "owned recovery token is outside the internal DFA route"), nil
	}
	// Non-EOF recovery also needs lexical-error reentry and bounded region
	// growth. Do not inherit the shared scheduler's accounting for those paths.
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte {
		return compactOwnedRecoveryDecline(index, "owned recovery currently requires EOF after the first absorb"), nil
	}
	source, err := s.s5RecoverySource()
	if err != nil {
		return nil, err
	}
	symbols := s.recoverySymbolPolicy()
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	current, supported, err := s.recoveryCondenseEntry(original, symbols, source, &memo)
	if err != nil {
		return nil, err
	}
	if !supported {
		return compactOwnedRecoveryDecline(index, "owned recovery cannot price the physical head"), nil
	}
	candidate, recoverable, err := s.ownedRecoverySummaryCandidate(index, current, symbols, source, &memo)
	if errors.Is(err, errDiagnosticParserCoreLineageCostUnavailable) {
		return compactOwnedRecoveryDecline(index, "owned recovery needs an unambiguous summary path"), nil
	}
	if err != nil {
		return nil, err
	}
	if recoverable && s.nextSeq == math.MaxUint64 {
		return nil, errors.New("parser-core phase zero: owned recovery sequence overflow")
	}
	// C can halt the absorber before EOF acceptance after a seventh version
	// appears. This bounded route declines before creating that frontier.
	if recoverable && len(s.headers) >= diagnosticParserCoreRecoveryVersionLimit {
		return compactOwnedRecoveryDecline(index, "owned EOF recovery requires an unmodeled version-cap transition"), nil
	}
	if s.token.Symbol == 0 {
		paths, err := s.compact.Derivations(original.head)
		if err != nil {
			return nil, err
		}
		if len(paths) != 1 || len(paths[0].Payloads)+len(region.children) > core.EOFAdmissionMaxTopPayloads {
			return compactOwnedRecoveryDecline(index, "owned recovery EOF needs one bounded path"), nil
		}
		for _, payloads := range [][]core.SubtreeID{paths[0].Payloads, region.children} {
			for _, payload := range payloads {
				view, err := s.compact.MaterializationView(payload)
				if err != nil {
					return nil, err
				}
				if view.Extra || view.Missing {
					return compactOwnedRecoveryDecline(index, "owned recovery EOF has an unsupported extra or missing payload"), nil
				}
			}
		}
	}
	cost, _, err := s.recoveryOutputCostFunc()
	if err != nil {
		return nil, err
	}
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	defer func() {
		if value := recover(); value != nil {
			snapshot.restore(s)
			panic(value)
		}
		if err != nil {
			snapshot.restore(s)
		}
	}()
	run := func(owner core.SchedulerTransactionToken) error {
		if err := s.reserveDispatches(1); err != nil {
			return err
		}
		if recoverable {
			head, err := s.compact.RecoverToAncestorStateWithOpenRegionAndCostOwned(
				owner, candidate, region.startByte, region.endByte, region.children, cost,
			)
			if err != nil {
				return err
			}
			recovered := original
			recovered.head = head
			recovered.creationSeq = s.nextSeq
			recovered.closeRecoveryRegion()
			recovered.shifted, recovered.paused, recovered.accepted = false, false, false
			recovered.markRecoveryLineage()
			recovered.markRecoveryCosted()
			s.nextSeq++
			s.headers = append(s.headers, recovered)
			s.work.add(&s.work.StackSummaryRecoveryForks, 1)
		}
		head, _, err := s.compact.RecoverEOFAcceptWithOpenRegionAndCostOwned(
			owner, original.head, region.startByte, region.endByte, region.children, cost,
		)
		if err != nil {
			return err
		}
		header := &s.headers[index]
		header.head = head
		header.closeRecoveryRegion()
		header.accepted, header.shifted, header.paused = true, false, false
		s.work.add(&s.work.Accepts, 1)
		s.work.add(&s.work.RecoverEOFAccepts, 1)
		s.invalidateVerifierHeaderBinding()
		s.epochProgress = true
		s.work.add(&s.work.Dispatches, 1)
		return nil
	}
	if s.freshSessionOwner != nil {
		err = run(*s.freshSessionOwner)
	} else {
		err = s.compact.ApplySchedulerAtomic(run)
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// Keep the C absorber beside its actionable stack-summary sibling.
func (s *diagnosticParserCoreGenericScheduler) dispatchFaithfulRecoveryRegion(index int) (handled bool, err error) {
	if !s.options.allowCompactFaithfulS5Recovery || !s.s4StackSummaryRecoveryAdmitted() || !s.options.allowCompactRecoveryErrorModeKeywordCapture || index < 0 || index >= len(s.headers) {
		return false, nil
	}
	original := s.headers[index]
	region := original.recoveryRegion()
	if region == nil || len(region.children) == 0 || s.token.Symbol == 0 || s.token.Symbol == errorSymbol || s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
		return false, nil
	}
	state, _, err := s.compact.Boundary(original.head)
	if err != nil || state != 0 {
		return false, err
	}
	token := s.token
	if relexed, ok := s.s3ErrorModeRelex(region.endByte); ok {
		if relexed.StartByte != token.StartByte || relexed.EndByte != token.EndByte || relexed.Symbol == errorSymbol {
			return false, nil
		}
		token = relexed
	}
	sharedToken := s.token
	s.token = token
	defer func() { s.token = sharedToken }()
	source, err := s.s5RecoverySource()
	if err != nil {
		return false, err
	}
	symbols := s.recoverySymbolPolicy()
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	current, supported, err := s.recoveryCondenseEntry(original, symbols, source, &memo)
	if err != nil || !supported {
		return false, err
	}
	candidate, recoverable, err := s.ownedRecoverySummaryCandidate(index, current, symbols, source, &memo)
	if errors.Is(err, errDiagnosticParserCoreLineageCostUnavailable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if recoverable && (len(s.headers) >= diagnosticParserCoreRecoveryVersionLimit || s.nextSeq == math.MaxUint64) {
		return false, nil
	}
	cost, _, err := s.recoveryOutputCostFunc()
	if err != nil {
		return false, err
	}
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	defer func() {
		if value := recover(); value != nil {
			snapshot.restore(s)
			panic(value)
		}
		if !handled {
			snapshot.restore(s)
		}
	}()
	run := func(owner core.SchedulerTransactionToken) error {
		if err := s.reserveDispatches(1); err != nil {
			return err
		}
		if recoverable {
			head, err := s.compact.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, region.startByte, region.endByte, region.children, cost)
			if err != nil {
				return err
			}
			resumed := original
			resumed.head = head
			resumed.creationSeq = s.nextSeq
			s.nextSeq++
			resumed.closeRecoveryRegion()
			resumed.shifted, resumed.paused, resumed.accepted = false, false, false
			resumed.markRecoveryLineage()
			resumed.markRecoveryCosted()
			s.headers = append(s.headers, resumed)
			s.work.add(&s.work.StackSummaryRecoveryForks, 1)
		}
		extra, err := s3TokenIsExtraShift(s.compact, token.Symbol)
		if err != nil {
			return err
		}
		leaf, err := s.compact.ErrorRegionLeaf(core.Symbol(token.Symbol), token.StartByte, token.EndByte, extra)
		if err != nil {
			return err
		}
		children := make([]core.SubtreeID, len(region.children)+1)
		copy(children, region.children)
		children[len(region.children)] = leaf
		header := &s.headers[index]
		header.setRecoveryRegion(&diagnosticParserCoreS3Region{state: region.state, startByte: region.startByte, endByte: token.EndByte, children: children})
		header.shifted = true
		header.clearZeroWidthReopened()
		s.invalidateVerifierHeaderBinding()
		s.epochProgress = true
		s.work.add(&s.work.Dispatches, 1)
		return s.persistHeaderLineageOwned(owner)
	}
	if s.freshSessionOwner != nil {
		err = run(*s.freshSessionOwner)
	} else {
		err = s.compact.ApplySchedulerAtomic(run)
	}
	handled = err == nil
	return handled, err
}

// A later owned version can enter another recovery while earlier versions
// have already consumed their tokens. Recover only this version and retain
// every sibling for the same cost competition used by C's summary scan.
func (s *diagnosticParserCoreGenericScheduler) tryFaithfulOwnedSummaryRecovery(index int) (handled bool, err error) {
	if !s.options.allowCompactFaithfulS5Recovery || !s.s4StackSummaryRecoveryAdmitted() || !s.versionLexerOwnershipActive || index < 0 || index >= len(s.headers) || len(s.headers) >= 6 {
		return false, nil
	}
	h := s.headers[index]
	if h.accepted || h.paused || h.recoveryRegion() != nil {
		return false, nil
	}
	request := s.versionLexerRequestForHeader(index)
	if request == nil || !request.valid || request.token.Symbol == 0 || request.token.Symbol == errorSymbol {
		return false, nil
	}
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	defer func() {
		if value := recover(); value != nil {
			snapshot.restore(s)
			panic(value)
		}
		if !handled {
			snapshot.restore(s)
		}
	}()
	original := diagnosticParserCoreS5CloneSlice(s.headers)
	companions := make([]diagnosticParserCoreHeader, 0, len(original)-1)
	for i, other := range original {
		if i != index {
			companions = append(companions, other)
		}
	}
	boundary, err := s.compact.ClassifyBoundary(h.head, core.Symbol(request.token.Symbol))
	if err != nil {
		return false, err
	}
	cell := diagnosticParserCoreGenericCell{headerIndex: int32(index), boundary: boundary, versionLexerRequest: h.versionLexerRequestReference()}
	err = s.withVersionLexerRequest(cell, func() error {
		// C halts the absorber when an earlier advanced version is already
		// cheaper than skipping this token. Keep its owned scanner cursor.
		if len(original) == 2 {
			otherIndex := 1 - index
			other := original[otherIndex]
			if other.shifted && !other.accepted && !other.paused && other.recoveryRegion() == nil && other.creationSeq < h.creationSeq && other.versionLexerSnapshot() != nil {
				src, err := s.s5RecoverySource()
				if err != nil {
					return err
				}
				symbols := s.recoverySymbolPolicy()
				var memo core.RecoveryCostMemo
				defer memo.Reset()
				current, supported, err := s.recoveryCondenseEntry(h, symbols, src, &memo)
				if err != nil || !supported {
					return err
				}
				start := uint32(request.before.dfa.lexerPos)
				cost := uint64(current.status.Cost) + core.RecoveryCostPerSkippedTree + uint64(request.token.EndByte-start)*core.RecoveryCostPerSkippedChar + uint64(src.rowAt(request.token.EndByte)-src.rowAt(start))*core.RecoveryCostPerSkippedLine
				if cost > math.MaxUint32 {
					return errors.New("parser-core phase zero: owned absorber cost overflow")
				}
				better, err := s.ownedRecoveryBetterVersionExists(index, current, uint32(cost), symbols, src, &memo)
				if err != nil {
					return err
				}
				if better {
					s.collapseToRecoveryWinner(otherIndex)
					handled = true
					if s.freshSessionOwner != nil {
						return s.persistHeaderLineageOwned(*s.freshSessionOwner)
					}
					return s.compact.ApplySchedulerAtomic(s.persistHeaderLineageOwned)
				}
			}
		}
		s.headers = []diagnosticParserCoreHeader{h}
		var recoveryErr error
		handled, recoveryErr = s.s4TryReductionFrontierSummaryRecovery(0, companions...)
		if recoveryErr != nil || !handled {
			return recoveryErr
		}
		if len(s.headers) < 1 || len(s.headers) > 2 {
			return errors.New("parser-core phase zero: owned summary recovery did not preserve its versions")
		}
		absorb := s.headers[0]
		var resume diagnosticParserCoreHeader
		hasResume := len(s.headers) == 2
		if hasResume {
			resume = s.headers[1]
		}
		publish := func(owner core.SchedulerTransactionToken) error {
			if err := s.publishVersionLexerShiftOnHeaderOwned(owner, &absorb, request); err != nil {
				return err
			}
			restored := diagnosticParserCoreS5CloneSlice(original)
			restored[index] = absorb
			if hasResume {
				restored = append(restored, resume)
			}
			s.headers = restored
			s.invalidateVerifierHeaderBinding()
			return s.persistHeaderLineageOwned(owner)
		}
		if s.freshSessionOwner != nil {
			return publish(*s.freshSessionOwner)
		}
		return s.compact.ApplySchedulerAtomic(publish)
	})
	if err != nil {
		handled = false
	}
	return handled, err
}

func (s *diagnosticParserCoreGenericScheduler) dispatchFaithfulOwnedRecoveryRegion(index int) (stop *diagnosticParserCoreGenericUnsupported, err error) {
	if err := s.requestHeaderLexerToken(index); err != nil {
		return nil, err
	}
	request := s.versionLexerRequestForHeader(index)
	if request == nil || !request.valid {
		return compactOwnedRecoveryDecline(index, "owned region has no authenticated token"), nil
	}
	boundary, err := s.compact.ClassifyBoundary(s.headers[index].head, core.Symbol(request.token.Symbol))
	if err != nil {
		return nil, err
	}
	cell := diagnosticParserCoreGenericCell{headerIndex: int32(index), boundary: boundary, versionLexerRequest: s.headers[index].versionLexerRequestReference()}
	err = s.withVersionLexerRequest(cell, func() error {
		if request.token.Symbol == 0 {
			var e error
			stop, e = s.dispatchOwnedRecoveryRegion(index)
			return e
		}
		handled, e := s.dispatchFaithfulRecoveryRegion(index)
		if e != nil {
			return e
		}
		if !handled {
			stop = compactOwnedRecoveryDecline(index, "owned region could not advance its internal token")
			return nil
		}
		publish := func(owner core.SchedulerTransactionToken) error {
			if err := s.publishVersionLexerShiftOnHeaderOwned(owner, &s.headers[index], request); err != nil {
				return err
			}
			return s.persistHeaderLineageOwned(owner)
		}
		if s.freshSessionOwner != nil {
			return publish(*s.freshSessionOwner)
		}
		return s.compact.ApplySchedulerAtomic(publish)
	})
	return stop, err
}
