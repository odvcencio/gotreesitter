//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// diagnosticParserCoreRecoveryTurns tracks C's physical advance round.
// Tokens and scanner cursors remain in the existing immutable version state.
type diagnosticParserCoreRecoveryTurns struct {
	active     bool
	index      int
	lastByte   uint32
	condensing bool
}

func (s *diagnosticParserCoreGenericScheduler) activateRecoveryVersionTurns() (err error) {
	if s.recoveryTurns.active && s.versionLexerOwnershipActive {
		return nil
	}
	if !s.recoveryTurns.active || !s.options.allowCompactRecoveryVersionTurns || !s.recoveryIsolation {
		return errors.New("parser-core phase zero: recovery turn activation lacks an admitted recovery frontier")
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
	if err = s.seedVersionLexerOwnershipMode(true); err != nil {
		return err
	}
	s.versionLexerOwnershipActive = true
	return nil
}

// dispatchRecoveryVersionTurn advances only the current physical version.
// A consumed token opens that version's next request, not a shared election.
func (s *diagnosticParserCoreGenericScheduler) dispatchRecoveryVersionTurn() (*diagnosticParserCoreGenericUnsupported, error) {
	if err := s.pollStopControl(); err != nil {
		return nil, err
	}
	if err := s.activateRecoveryVersionTurns(); err != nil {
		return nil, err
	}
	if s.recoveryTurns.index >= len(s.headers) {
		return s.finishRecoveryVersionRound()
	}
	s.pruneRecoveryVersionLexerRequests()
	index := s.recoveryTurns.index
	header := &s.headers[index]
	if header.accepted || header.paused {
		s.recoveryTurns.index++
		return nil, nil
	}
	if header.shifted {
		if header.versionLexerRequestReference() != 0 {
			return nil, errors.New("parser-core phase zero: consumed recovery version retains its lookahead request")
		}
		previous := *header
		header.shifted = false
		header.frontierSequence = 0
		if err := s.requestHeaderLexerToken(index); err != nil {
			*header = previous
			return nil, err
		}
	} else if err := s.requestHeaderLexerToken(index); err != nil {
		return nil, err
	}
	request := s.versionLexerRequestForHeader(index)
	if request == nil {
		return nil, errors.New("parser-core phase zero: recovery turn has no authenticated lookahead")
	}
	// No-lookahead closure needs a separate per-version progress proof.
	if request.token.NoLookahead {
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "recovery turn reached an unsupported no-lookahead token", headerIndex: index,
		}, nil
	}
	var stop *diagnosticParserCoreGenericUnsupported
	var err error
	if header.recoveryRegion() != nil {
		boundary, boundaryErr := s.compact.ClassifyBoundary(header.head, core.Symbol(request.token.Symbol))
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		cell := diagnosticParserCoreGenericCell{
			headerIndex: int32(index), boundary: boundary, versionLexerRequest: header.versionLexerRequestReference(),
		}
		err = s.withVersionLexerRequest(cell, func() error {
			var dispatchErr error
			stop, dispatchErr = s.dispatchOwnedRecoveryRegion(index)
			return dispatchErr
		})
	} else {
		stop, err = s.dispatchPass()
		if err == nil && stop != nil && stop.boundary == DiagnosticParserCoreRecovery &&
			stop.detail == diagnosticParserCoreNoTableActionDetail {
			supported, baselineErr := s.resetRecoveryNodeBaseline(&s.headers[index])
			if baselineErr != nil {
				return nil, baselineErr
			}
			if !supported {
				return stop, nil
			}
			s.headers[index].paused = true
			stop = nil
		}
	}
	if err != nil || stop != nil {
		return stop, err
	}
	if index >= len(s.headers) {
		return nil, errors.New("parser-core phase zero: recovery advance removed its physical slot before condensation")
	}
	header = &s.headers[index]
	_, position, err := s.compact.Boundary(header.head)
	if err != nil {
		return nil, err
	}
	if region := header.recoveryRegion(); region != nil {
		position = region.endByte
	}
	// Match parser.c's advance-loop break condition after the operation.
	if position > s.recoveryTurns.lastByte || (index > 0 && position == s.recoveryTurns.lastByte) {
		s.recoveryTurns.lastByte = position
		s.recoveryTurns.index++
	} else if header.accepted || header.paused {
		s.recoveryTurns.index++
	}
	return nil, nil
}

func (s *diagnosticParserCoreGenericScheduler) finishRecoveryVersionRound() (stop *diagnosticParserCoreGenericUnsupported, err error) {
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
	s.recoveryTurns.condensing = true
	err = s.withVersionLexerOwner(func(owner core.SchedulerTransactionToken) error {
		return s.canonicalizeOwned(owner)
	})
	s.recoveryTurns.condensing = false
	if err != nil {
		return nil, err
	}
	accepted, runnable := 0, 0
	for index := range s.headers {
		if s.headers[index].accepted {
			accepted++
		} else if !s.headers[index].paused {
			runnable++
		}
	}
	complete := accepted == len(s.headers) && accepted != 0
	if !complete && accepted != 0 && runnable == 0 {
		var err error
		complete, err = s.finishedRecoveryTreeBeatsPausedFrontier()
		if err != nil {
			return nil, err
		}
	}
	if complete {
		index := 0
		for !s.headers[index].accepted {
			index++
		}
		request := s.versionLexerRequestForHeader(index)
		if request == nil {
			return nil, errors.New("parser-core phase zero: accepted recovery version lost its EOF request")
		}
		boundary, err := s.compact.ClassifyBoundary(s.headers[index].head, core.Symbol(request.token.Symbol))
		if err != nil {
			return nil, err
		}
		cell := diagnosticParserCoreGenericCell{
			headerIndex: int32(index), boundary: boundary, versionLexerRequest: s.headers[index].versionLexerRequestReference(),
		}
		return nil, s.withVersionLexerRequest(cell, s.completeAcceptance)
	}
	if runnable == 0 {
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRecovery, detail: "recovery turn requires a paused-version resume before acceptance",
		}, nil
	}
	if len(s.headers) == 1 && s.headers[0].shifted && s.headers[0].recoveryRegion() == nil {
		if err := s.rejoinSharedLexerFromOwnedHeader(); err != nil {
			return nil, err
		}
		s.recoveryTurns = diagnosticParserCoreRecoveryTurns{}
		return nil, nil
	}
	if err := s.compact.BeginFrontier(); err != nil {
		return nil, err
	}
	s.epochProgress = false
	s.recoveryTurns.index = 0
	return nil, nil
}

// C's condense tail resumes the best paused version and returns its stored
// stack cost, before recovery changes it. A cheaper finished tree ends parsing.
// This path proves that outcome without constructing versions C then discards.
func (s *diagnosticParserCoreGenericScheduler) finishedRecoveryTreeBeatsPausedFrontier() (bool, error) {
	source, err := s.s5RecoverySource()
	if err != nil {
		return false, err
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	var pausedCost uint32
	pausedFound := false
	var finishedCost uint32
	finishedFound := false
	for index := range s.headers {
		header := s.headers[index]
		if header.accepted && header.recoveryRegion() != nil {
			return false, nil
		}
		if !header.accepted && !header.paused {
			return false, nil
		}
		entry, supported, err := s.recoveryCondenseEntry(header, symbols, source, &memo)
		if err != nil || !supported {
			return false, err
		}
		if header.accepted {
			if !finishedFound || entry.status.Cost < finishedCost {
				finishedCost, finishedFound = entry.status.Cost, true
			}
		} else if !pausedFound {
			// key.cost excludes the paused-status skipped-tree surcharge.
			pausedCost, pausedFound = entry.key.cost, true
		}
	}
	return finishedFound && pausedFound && finishedCost < pausedCost, nil
}

// Keep only requests still owned by a live or accepted version.
// Run outside request callbacks, which temporarily retain pointers into this slice.
func (s *diagnosticParserCoreGenericScheduler) pruneRecoveryVersionLexerRequests() {
	write := 0
	for read := range s.versionLexerRequests {
		reference := uint32(read + 1)
		used := false
		for index := range s.headers {
			header := &s.headers[index]
			if header.versionLexerRequestReference() != reference {
				continue
			}
			used = true
			if write != read {
				baseline, baselineSet := header.recoveryNodeBaseline()
				header.publishVersionState(header.recoveryRegion(), header.versionLexerSnapshot(),
					uint32(write+1), header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(), baseline, baselineSet)
			}
		}
		if used {
			s.versionLexerRequests[write] = s.versionLexerRequests[read]
			write++
		}
	}
	clear(s.versionLexerRequests[write:])
	s.versionLexerRequests = s.versionLexerRequests[:write]
}
