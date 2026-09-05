//go:build !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

// mergeRecoveredReductionSiblingOwned preserves C's existing sibling order.
// Equal stored costs permit a merge; they do not select an ambiguity winner.
func (s *diagnosticParserCoreGenericScheduler) mergeRecoveredReductionSiblingOwned(
	owner core.SchedulerTransactionToken,
	sourceIndex int,
	incoming diagnosticParserCoreHeader,
) (bool, error) {
	if incoming.recoveryRegion() != nil || incoming.paused || incoming.accepted {
		return false, nil
	}
	state, position, err := s.compact.Boundary(incoming.head)
	if err != nil {
		return false, err
	}
	cost, err := s.compact.RecoveryStoredErrorCost(incoming.head)
	if err != nil || cost == 0 {
		return false, err
	}
	key := diagnosticParserCoreRecoveryCondenseKey{
		state: state, byteOffset: position, cost: cost,
		checkpoint: incoming.checkpoint, shifted: incoming.shifted,
	}
	for index := range s.headers {
		if index == sourceIndex {
			continue
		}
		sibling := s.headers[index]
		if sibling.accepted || sibling.paused || sibling.recoveryRegion() != nil ||
			sibling.shifted != incoming.shifted || sibling.checkpoint != incoming.checkpoint ||
			!s.versionLexerStateEqual(sibling.versionState, incoming.versionState) {
			continue
		}
		siblingState, siblingPosition, err := s.compact.Boundary(sibling.head)
		if err != nil {
			return false, err
		}
		if siblingState != state || siblingPosition != position {
			continue
		}
		siblingCost, err := s.compact.RecoveryStoredErrorCost(sibling.head)
		if err != nil {
			return false, err
		}
		if siblingCost != cost {
			continue
		}
		source, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
		if err != nil {
			return false, err
		}
		var memo core.RecoveryCostMemo
		target := diagnosticParserCoreRecoveryCondenseEntry{header: sibling, key: key}
		candidate := diagnosticParserCoreRecoveryCondenseEntry{header: incoming, key: key}
		merged, err := s.mergeEquivalentRecoveryEntriesOwned(owner, &target, candidate,
			diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language), source, &memo, true)
		memo.Reset()
		if err != nil {
			return false, err
		}
		if merged {
			if err := s.compact.RecordDropCohortProducerMutation(owner, core.DropCohortProducerSiblingAdoption); err != nil {
				return false, err
			}
			s.headers[index] = target.header
			return true, nil
		}
	}
	return false, nil
}
