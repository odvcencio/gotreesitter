//go:build !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

// lexicalErrorResumeMatchesSummary checks the first eligible recovery state.
// A merged marker must not resume through a later closure history.
func (s *diagnosticParserCoreGenericScheduler) lexicalErrorResumeMatchesSummary(
	header diagnosticParserCoreHeader, lookahead Symbol,
) (bool, error) {
	state, markerByte, err := s.compact.Boundary(header.head)
	if err != nil || state != 0 || s.recoveryIsolation {
		return err == nil, err
	}
	region := header.recoveryRegion()
	if region == nil || len(s.headers) != 1 {
		return false, nil
	}
	candidates, err := s.compact.StackSummaryCandidates(header.head, cRecoverMaxSummaryDepth)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if candidate.State() == 0 || candidate.ByteOffset() >= region.endByte {
			continue
		}
		row, err := s.compact.Actions(candidate.State(), core.Symbol(lookahead))
		if err != nil {
			return false, err
		}
		if row.Len() == 0 {
			continue
		}
		return candidate.Depth() == 1 && candidate.ByteOffset() == markerByte && candidate.State() == region.state, nil
	}
	return false, nil
}

// tryLexicalErrorRecovery closes every reduction before opening one error region.
// It does not enable missing-token insertion or recovery version competition.
func (s *diagnosticParserCoreGenericScheduler) tryLexicalErrorRecovery(index int) (bool, error) {
	if s == nil || s.compact == nil || !s.s3ErrorRegionAdmitted() ||
		len(s.headers) != 1 || index != 0 || s.token.Symbol != errorSymbol ||
		s.token.Missing || s.token.NoLookahead || s.token.EndByte <= s.token.StartByte ||
		s.tokenSource == nil || s.tokenSource.language == nil || s.tokenSource.lexer == nil ||
		s.headers[0].recoveryRegion() != nil || s.s3RegionOpened || s.s3ResumeCount != 0 {
		return false, nil
	}
	if s.token.StartByte <= firstNonTriviaByteStart(s.tokenSource.lexer.source) {
		return false, nil
	}
	// The synthetic error symbol cannot be a grammar lookahead.
	if s.tokenSource.language.TokenCount <= 1 || uint32(s.tokenSource.language.TokenCount) > uint32(errorSymbol) {
		return false, nil
	}
	if relexed, ok := s.s3ErrorModeRelex(s.token.StartByte); ok && relexed.Symbol != errorSymbol &&
		(relexed.Symbol != s.token.Symbol || relexed.EndByte != s.token.EndByte) {
		return false, nil
	}
	handled, err := s.s5TryRecoveryTransaction(index, true)
	if err != nil || !handled {
		return handled, err
	}
	// The region is open and committed. A lexical error region never starts
	// recovery version turns, and a language whose recovered trees publish
	// only through an executed EOF turn therefore cannot publish this parse.
	// Decline now instead of parsing to end of file and declining at
	// materialization (issue #454: a mid-file Go error paid a full compact
	// pass before falling back to production).
	if err := s.declineUnpublishableSharedRecovery(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) runLexicalErrorRecoveryOwned(
	owner core.SchedulerTransactionToken,
	index int,
	staged *diagnosticParserCoreS5Work,
) (bool, error) {
	_, position, err := s.compact.Boundary(s.headers[index].head)
	if err != nil || position > s.token.StartByte {
		return false, err
	}
	if _, err := s.s5RunReductionFrontierOwned(owner, index, diagnosticParserCoreS5AnyTerminal, core.Symbol(s.token.Symbol), staged); err != nil {
		return false, err
	}
	frontier := diagnosticParserCoreS5CloneSlice(s.headers)
	if len(frontier) == 0 {
		return false, nil
	}
	baseline, supported, err := s.s5RecoveryBaseline(frontier)
	if err != nil || !supported {
		return false, err
	}
	absorb, err := s.s5AppendAndMergeAbsorberOwned(owner, frontier, baseline, 0, staged)
	if err != nil || absorb.head.Node == 0 {
		return false, err
	}
	absorb.clearRecoveryLineage()
	clear(s.headers)
	s.headers = append(s.headers[:0], absorb)
	s.recoveryIsolation = false
	s.epochProgress = true
	if err := s.canonicalizeOwned(owner); err != nil {
		return false, err
	}
	if err := s.persistHeaderLineageOwned(owner); err != nil {
		return false, err
	}
	return true, nil
}
