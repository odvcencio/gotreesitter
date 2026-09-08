//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
	"math"
)

func (s *diagnosticParserCoreGenericScheduler) advanceRecoveryEOFOwned(owner core.SchedulerTransactionToken, index int, original diagnosticParserCoreHeader, candidate core.StackSummaryCandidate, recoverable bool, cost core.ReductionOutputCostFunc) error {
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	region := original.recoveryRegion()
	request := s.versionLexerRequestForHeader(index)
	if recoverable {
		var heads []core.Head
		var err error
		if len(region.children) == 0 {
			heads, err = s.compact.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, cost)
		} else {
			heads, err = s.compact.RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned(owner, candidate, region.startByte, region.endByte, region.children, cost)
		}
		if err != nil {
			return err
		}
		for _, head := range heads {
			if s.nextSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: EOF recovery sequence overflow")
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
		if len(heads) != 0 && len(s.headers) > diagnosticParserCoreRecoveryVersionLimit {
			return s.haltRecoveryTokenVersion(index)
		}
	}
	heads, err := s.compact.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, original.head, region.startByte, region.endByte, region.children, cost)
	if err != nil {
		return err
	}
	for i, head := range heads {
		header := original
		header.head = head
		header.closeRecoveryRegion()
		header.accepted, header.shifted, header.paused = true, false, false
		slot := index
		if i == 0 {
			s.headers[index] = header
		} else {
			if s.nextSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: EOF acceptance sequence overflow")
			}
			header.creationSeq = s.nextSeq
			s.nextSeq++
			slot = len(s.headers)
			s.headers = append(s.headers, header)
		}
		if err := s.recordRecoveryAcceptance(slot); err != nil {
			return err
		}
		s.work.add(&s.work.Accepts, 1)
		s.work.add(&s.work.RecoverEOFAccepts, 1)
	}
	s.invalidateVerifierHeaderBinding()
	s.epochProgress = true
	s.work.add(&s.work.Dispatches, 1)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) beginRecoveryEOFOwned(owner core.SchedulerTransactionToken) error {
	source, err := s.s5RecoverySource()
	if err != nil {
		return err
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	current, supported, err := s.recoveryCondenseEntry(s.headers[0], symbols, source, &memo)
	if err != nil {
		return err
	}
	if !supported {
		return diagnosticParserCoreLineageCostUnavailable
	}
	candidate, recoverable, err := s.ownedRecoverySummaryCandidate(0, current, symbols, source, &memo)
	if err != nil {
		return err
	}
	cost, _, err := s.s5RecoveryOutputCostFunc()
	if err != nil {
		return err
	}
	return s.advanceRecoveryEOFOwned(owner, 0, s.headers[0], candidate, recoverable, cost)
}
