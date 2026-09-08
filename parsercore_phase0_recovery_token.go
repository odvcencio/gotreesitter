//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func (s *diagnosticParserCoreGenericScheduler) advanceRecoveryToken(index int) (err error) {
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
	run := func(owner core.SchedulerTransactionToken) error { return s.advanceRecoveryTokenOwned(owner, index) }
	if s.freshSessionOwner != nil {
		return run(*s.freshSessionOwner)
	}
	return s.compact.ApplySchedulerAtomic(run)
}

func (s *diagnosticParserCoreGenericScheduler) haltRecoveryTokenVersion(index int) error {
	if index < 0 || index >= 128 {
		return errors.New("parser-core phase zero: recovery halt index exceeds capacity")
	}
	s.recoveryTurns.halted[index/64] |= uint64(1) << uint(index%64)
	s.headers[index].paused = true
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) advanceRecoveryTokenOwned(owner core.SchedulerTransactionToken, index int) error {
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	s.work.add(&s.work.Dispatches, 1)
	original := s.headers[index]
	region := original.recoveryRegion()
	request := s.versionLexerRequestForHeader(index)
	if region == nil || s.token.Symbol == 0 || s.token.EndByte < s.token.StartByte || s.token.StartByte < region.endByte ||
		s.token.EndByte == s.token.StartByte {
		return errors.New("parser-core phase zero: recovery advance requires a real token after its region")
	}
	source, err := s.s5RecoverySource()
	if err != nil {
		return err
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	current, supported, err := s.recoveryCondenseEntry(original, symbols, source, &memo)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("recovery advance state pricing: %w", diagnosticParserCoreLineageCostUnavailable)
	}
	candidate, recoverable, err := s.ownedRecoverySummaryCandidate(index, current, symbols, source, &memo)
	if err != nil {
		return err
	}
	cost, _, err := s.s5RecoveryOutputCostFunc()
	if err != nil {
		return err
	}
	if recoverable {
		var outputs []core.Head
		if len(region.children) == 0 {
			outputs, err = s.compact.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, cost)
		} else {
			outputs, err = s.compact.RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned(owner, candidate, region.startByte, region.endByte, region.children, cost)
		}
		if err != nil {
			return err
		}
		for _, head := range outputs {
			if s.nextSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: recovery sequence overflow")
			}
			recovered := original
			recovered.head, recovered.creationSeq = head, s.nextSeq
			s.nextSeq++
			recovered.closeRecoveryRegion()
			recovered.shifted, recovered.paused, recovered.accepted = false, false, false
			recovered.markRecoveryLineage()
			recovered.markRecoveryCosted()
			if request != nil {
				if err := s.installEquivalentVersionLexerState(&recovered, request.before, 0, nil); err != nil {
					return err
				}
			}
			s.headers = append(s.headers, recovered)
			s.work.add(&s.work.StackSummaryRecoveryForks, 1)
		}
		scannerChanged := s.token.ExternalScannerToken && s.checkpointBeforeID != s.checkpointID
		if request != nil {
			scannerChanged = request.beforeID != request.afterID
		}
		if len(outputs) != 0 && (len(s.headers) > diagnosticParserCoreRecoveryVersionLimit || scannerChanged) {
			return s.haltRecoveryTokenVersion(index)
		}
	}
	newCost := uint64(current.status.Cost) + core.RecoveryCostPerSkippedTree +
		uint64(s.token.EndByte-region.endByte)*core.RecoveryCostPerSkippedChar +
		uint64(source.rowAt(s.token.EndByte)-source.rowAt(region.endByte))*core.RecoveryCostPerSkippedLine
	if newCost > math.MaxUint32 {
		return errors.New("parser-core phase zero: recovery absorption cost overflow")
	}
	better, err := s.ownedRecoveryBetterVersionExists(index, current, uint32(newCost), symbols, source, &memo)
	if err != nil {
		return err
	}
	if better {
		return s.haltRecoveryTokenVersion(index)
	}
	extra, err := s3TokenIsExtraShift(s.compact, s.token.Symbol)
	if err != nil {
		return err
	}
	leaf, err := s.compact.ErrorRegionLeaf(core.Symbol(s.token.Symbol), s.token.StartByte, s.token.EndByte, extra)
	if err != nil {
		return err
	}
	children := append(append([]core.SubtreeID(nil), region.children...), leaf)
	header := &s.headers[index]
	header.openRecoveryRegion(&diagnosticParserCoreS3Region{state: region.state, startByte: region.startByte, endByte: s.token.EndByte, children: children})
	header.shifted, header.paused = true, false
	if request != nil {
		if err := s.publishVersionLexerShiftOnHeaderOwned(owner, header, request); err != nil {
			return err
		}
	}
	s.epochProgress = true
	s.s3RegionOpened = true
	return s.persistHeaderLineageOwned(owner)
}

func (s *diagnosticParserCoreGenericScheduler) beginRecoveryFrontierOwned(owner core.SchedulerTransactionToken, frontier []diagnosticParserCoreHeader, staged *diagnosticParserCoreS5Work) (bool, error) {
	return s.beginRecoveryFrontierOwnedWithRemainder(owner, frontier, staged, nil)
}

func (s *diagnosticParserCoreGenericScheduler) beginRecoveryFrontierOwnedWithRemainder(owner core.SchedulerTransactionToken, frontier []diagnosticParserCoreHeader, staged *diagnosticParserCoreS5Work, remainder []diagnosticParserCoreHeader) (bool, error) {
	baseline, supported, err := s.s5RecoveryBaseline(frontier)
	if err != nil || !supported {
		return false, err
	}
	marker, err := s.s5MergeRecoveryMarkerOwned(owner, frontier, staged)
	if err != nil || marker.head.Node == 0 {
		return false, err
	}
	_, position, err := s.compact.Boundary(marker.head)
	if err != nil {
		return false, err
	}
	marker.openRecoveryRegion(&diagnosticParserCoreS3Region{startByte: s.token.StartByte, endByte: position})
	marker.publishRecoveryCondenseState(0, 0, baseline, true)
	marker.markRecoveryLineage()
	marker.markRecoveryCosted()
	clear(s.headers)
	s.headers = append(s.headers[:0], marker)
	s.headers = appendRecoveryEpisodeRemainder(s.headers, remainder)
	s.recoveryIsolation = true
	s.options.allowCompactRecoveryVersionTurns = true
	s.recoveryTurns = diagnosticParserCoreRecoveryTurns{active: true, lastByte: position}
	if s.token.Symbol == 0 {
		s.headers[0].openRecoveryRegion(&diagnosticParserCoreS3Region{startByte: position, endByte: position})
		return true, s.beginRecoveryEOFOwned(owner)
	}
	if err := s.advanceRecoveryTokenOwned(owner, 0); err != nil {
		return false, err
	}
	return true, nil
}

func appendRecoveryEpisodeRemainder(dst, remainder []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	for _, header := range remainder {
		header.markRecoveryLineage()
		header.markRecoveryCosted()
		dst = append(dst, header)
	}
	return dst
}
