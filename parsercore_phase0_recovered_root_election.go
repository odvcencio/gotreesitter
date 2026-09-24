//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type recoveredRootGroup struct {
	recovery uint64
	missing  uint64
}

func recoveredRootGroupOf(header diagnosticParserCoreHeader) recoveredRootGroup {
	return recoveredRootGroup{
		recovery: header.recoveryGroupIdentity(),
		missing:  header.recoveryMissingGroupIdentity(),
	}
}

func (group recoveredRootGroup) valid() bool {
	return group.recovery != 0 || group.missing != 0
}

// electRecoveredRoot folds the winning recovery group's paths in C pop order.
// A missing source, open region, unsupported root, or excessive path count
// declines the compact route rather than publishing a guessed tree.
func (s *diagnosticParserCoreGenericScheduler) electRecoveredRoot(
	header diagnosticParserCoreHeader, paths []core.Derivation,
) (int, bool, error) {
	if s == nil || s.compact == nil || len(paths) < 2 ||
		len(paths) > compactAcceptanceElectionMaxLiveDerivations ||
		!s.options.allowCompactRecoveryLineageSelection ||
		!header.isRecoveryLineage() || !header.isRecoveryCosted() ||
		!s.s3RegionOpened && s.s5MissingInsertions == 0 &&
			s.work.RecoveryLineageSelections == 0 && s.work.RecoveryAmbiguityForks == 0 ||
		len(s.options.materializationSource) == 0 ||
		header.recoveryRegion() != nil {
		return 0, false, nil
	}
	src, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
	if err != nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return 0, false, nil
	}
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	symbols := s.recoverySymbolPolicy()
	open := header.recoveryOpenSegments()
	if open < 0 || uint64(open)*uint64(core.RecoveryCostPerRecovery) > math.MaxUint32 {
		return 0, false, nil
	}
	openCost := uint32(open) * core.RecoveryCostPerRecovery
	costs := make([]uint32, len(paths))
	for i, path := range paths {
		// C rebuilds the accepted root from the complete payload list.
		if len(path.Payloads) == 0 {
			return 0, false, nil
		}
		cost, priceErr := diagnosticParserCoreDerivationErrorCost(symbols, src, &memo, path)
		if priceErr != nil || cost > math.MaxUint32-openCost {
			return 0, false, nil
		}
		costs[i] = cost + openCost
	}
	winner := 0
	for candidate := 1; candidate < len(paths); candidate++ {
		switch {
		case costs[candidate] < costs[winner]:
			winner = candidate
		case costs[candidate] > costs[winner]:
			continue
		case paths[candidate].Score > paths[winner].Score:
			winner = candidate
		case paths[candidate].Score < paths[winner].Score:
			continue
		case costs[candidate] > 0:
			winner = candidate
		default:
			// The clean comparator requires one already rebuilt root on each
			// path. A multi-payload clean root needs its own bounded builder.
			if len(paths[winner].Payloads) != 1 || len(paths[candidate].Payloads) != 1 {
				return 0, false, nil
			}
			comparison, compareErr := s.compact.CompareCSelectionSubtrees(
				paths[winner].Payloads[0], paths[candidate].Payloads[0],
			)
			if errors.Is(compareErr, core.ErrCSelectionComparisonBudget) {
				return 0, false, nil
			}
			if compareErr != nil {
				return 0, false, compareErr
			}
			if comparison > 0 {
				winner = candidate
			}
		}
	}
	return winner, true, nil
}
