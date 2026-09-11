//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func (s *diagnosticParserCoreGenericScheduler) pauseRecoveryVersion(index int) error {
	header := &s.headers[index]
	baseline, supported, err := s.s5RecoveryBaseline([]diagnosticParserCoreHeader{*header})
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("paused recovery baseline: %w", diagnosticParserCoreLineageCostUnavailable)
	}
	if region := header.recoveryRegion(); region != nil {
		source, err := s.s5RecoverySource()
		if err != nil {
			return err
		}
		count, err := diagnosticParserCoreOpenRegionVisibleNodeCount(diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language), source, region)
		if err != nil {
			return err
		}
		if math.MaxUint32-baseline < count {
			return errors.New("parser-core phase zero: paused recovery baseline overflow")
		}
		baseline += count
	}
	header.publishRecoveryCondenseState(header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(), baseline, true)
	header.paused = true
	return nil
}

// recoveryPausedResumeIndex reads C's already-condensed status order.
// Accepted headers belong to the finished-tree pool, not the live stack.
func (s *diagnosticParserCoreGenericScheduler) recoveryPausedResumeIndex() int {
	if s.work.Accepts >= diagnosticParserCoreRecoveryVersionLimit {
		return -1
	}
	for index, header := range s.headers {
		if header.accepted {
			continue
		}
		if header.paused {
			return index
		}
		return -1
	}
	return -1
}

func (s *diagnosticParserCoreGenericScheduler) resumePausedRecoveryOwned(owner core.SchedulerTransactionToken) (err error) {
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
	return s.compact.RunSchedulerOwned(owner, func() error { return s.resumePausedRecoveryUncheckpointed(owner) })
}

func (s *diagnosticParserCoreGenericScheduler) resumePausedRecovery() error {
	run := func() error { return s.withVersionLexerOwner(s.resumePausedRecoveryOwned) }
	index := s.recoveryPausedResumeIndex()
	if index < 0 {
		return run()
	}
	request := s.versionLexerRequestForHeader(index)
	if request == nil {
		return errors.New("parser-core phase zero: paused recovery lacks its owned lookahead")
	}
	boundary, err := s.compact.ClassifyBoundary(s.headers[index].head, core.Symbol(request.token.Symbol))
	if err != nil {
		return err
	}
	cell := diagnosticParserCoreGenericCell{headerIndex: int32(index), boundary: boundary, versionLexerRequest: s.headers[index].versionLexerRequestReference()}
	return s.withVersionLexerRequest(cell, run)
}

func (s *diagnosticParserCoreGenericScheduler) resumePausedRecoveryUncheckpointed(owner core.SchedulerTransactionToken) error {
	index := s.recoveryPausedResumeIndex()
	if index >= 0 {
		original := s.headers[index]
		request := s.versionLexerRequestForHeader(index)
		if request == nil {
			return errors.New("parser-core phase zero: paused recovery lacks its owned lookahead")
		}
		remainder := append([]diagnosticParserCoreHeader(nil), s.headers[:index]...)
		remainder = append(remainder, s.headers[index+1:]...)
		s.headers = []diagnosticParserCoreHeader{original}
		s.headers[0].paused = false
		if region := original.recoveryRegion(); region != nil {
			if len(region.children) != 0 {
				cost, _, err := s.s5RecoveryOutputCostFunc()
				if err != nil {
					return err
				}
				head, err := s.compact.PushRecoveryErrorRepeatOwned(owner, original.head, region.children, cost)
				if err != nil {
					return err
				}
				s.headers[0].head = head
			}
			s.headers[0].closeRecoveryRegion()
		}
		staged := diagnosticParserCoreS5Work{}
		err := func() error {
			var handled bool
			var err error
			if s.token.Symbol == errorSymbol {
				if _, err = s.s5RunReductionFrontierOwned(owner, 0, diagnosticParserCoreS5AnyTerminal, core.Symbol(s.token.Symbol), &staged); err != nil {
					return err
				}
				frontier := append([]diagnosticParserCoreHeader(nil), s.headers...)
				handled, err = s.beginRecoveryFrontierOwnedWithRemainder(owner, frontier, &staged, remainder)
			} else {
				handled, err = s.s5RunOwnedWithRemainder(owner, 0, &staged, remainder)
			}
			if err != nil {
				return err
			}
			if !handled {
				return errors.New("parser-core phase zero: paused recovery could not publish its complete frontier")
			}
			return nil
		}()
		if err != nil {
			return err
		}
		s.commitS5Work(staged)
	}
	// C visits only the old live slots after handle_error. New absorber
	// siblings can already be halted and must keep their halt marker.
	write := 0
	var halted [2]uint64
	for read, header := range s.headers {
		isHalted := read < 128 && s.recoveryTurns.halted[read/64]&(uint64(1)<<uint(read%64)) != 0
		if header.paused && !header.accepted && !isHalted {
			continue
		}
		s.headers[write] = header
		if isHalted {
			halted[write/64] |= uint64(1) << uint(write%64)
		}
		write++
	}
	clear(s.headers[write:])
	s.headers = s.headers[:write]
	s.recoveryTurns.halted = halted
	s.invalidateVerifierHeaderBinding()
	return s.persistHeaderLineageOwned(owner)
}
