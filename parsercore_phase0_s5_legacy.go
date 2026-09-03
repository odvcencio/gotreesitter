//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

const (
	s5MissingTokenMaxSpineDepth  = 4096
	s5MissingTokenMaxSimulations = 256
)

// s5TryMissingTokenInsertionLegacy forks one no-action head into two versions.
// The original version absorbs the real token through S3. Its copied sibling
// inserts C's lowest viable missing terminal. This order matches C's version
// list, which is load-bearing for equal-cost recovery selection.
func (s *diagnosticParserCoreGenericScheduler) s5TryMissingTokenInsertionLegacy(index int) (handled bool, err error) {
	if !s.s5MissingTokenAdmitted() || len(s.headers) != 1 || index != 0 {
		return false, nil
	}
	original := s.headers[index]
	if original.recoveryRegion() != nil || s.token.Missing || s.token.NoLookahead ||
		s.token.Symbol == errorSymbol || s.s5MissingInsertions >= maxDiagnosticParserCoreMissingInsertions {
		return false, nil
	}
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return false, nil
	}
	tokenCount := s.tokenSource.language.TokenCount
	if tokenCount <= 1 || tokenCount > uint32(math.MaxUint16) {
		return false, nil
	}

	// C closes productions before it scans for a missing terminal. Keep the
	// original header unchanged unless the complete two-version fork succeeds.
	closedHead, closedChanged, closeOK, closeErr := s.s3CloseInProgressProductions(original.head)
	if closeErr != nil {
		return false, closeErr
	}
	if !closeOK {
		return false, nil
	}
	scanHead := original.head
	if closedChanged {
		scanHead = closedHead
	}
	state, byteOffset, boundaryErr := s.compact.Boundary(scanHead)
	if boundaryErr != nil {
		return false, boundaryErr
	}
	if byteOffset > s.token.StartByte {
		return false, nil
	}
	spine, spineOK, spineErr := s.compact.UniqueStateSpine(scanHead, s5MissingTokenMaxSpineDepth)
	if spineErr != nil {
		return false, spineErr
	}
	if !spineOK || len(spine) == 0 || spine[len(spine)-1] != state {
		return false, nil
	}

	simulation := make([]core.StateID, len(spine)+1)
	copy(simulation, spine)
	var missing Symbol
	var targetState core.StateID
	simulations := 0
	for rawSymbol := uint32(1); rawSymbol < tokenCount; rawSymbol++ {
		if rawSymbol%64 == 0 {
			if pollErr := s.pollStopControl(); pollErr != nil {
				return false, pollErr
			}
		}
		candidate := Symbol(rawSymbol)
		row, rowErr := s.compact.Actions(state, core.Symbol(candidate))
		if rowErr != nil {
			return false, rowErr
		}
		if row.Len() == 0 {
			continue
		}
		last := row.At(row.Len() - 1)
		if last.Type != core.ActionShift {
			continue
		}
		nextState := core.StateID(last.State)
		if last.Extra {
			nextState = state
		}
		if nextState == 0 || nextState == state {
			continue
		}
		nextRow, nextErr := s.compact.Actions(nextState, core.Symbol(s.token.Symbol))
		if nextErr != nil {
			return false, nextErr
		}
		if nextRow.Len() == 0 || nextRow.At(0).Type != core.ActionReduce {
			continue
		}
		simulations++
		if simulations > s5MissingTokenMaxSimulations {
			return false, nil
		}
		simulation[len(simulation)-1] = nextState
		canShift, shiftErr := s.compact.CanShiftAfterReductions(simulation, core.Symbol(s.token.Symbol))
		if shiftErr != nil {
			return false, shiftErr
		}
		if !canShift {
			continue
		}
		// This materializer cannot propagate the error bit after a hidden,
		// childless missing terminal is spliced out. C selects the first viable
		// symbol, so decline the whole scan instead of trying a later symbol.
		if index := int(candidate); index < len(s.tokenSource.language.SymbolMetadata) &&
			!s.tokenSource.language.SymbolMetadata[index].Visible {
			return false, nil
		}
		missing = candidate
		targetState = nextState
		break
	}
	if missing == 0 {
		return false, nil
	}
	if s.nextSeq == math.MaxUint64 {
		return false, errors.New("parser-core phase zero: recovery fork creation sequence overflow")
	}
	recoveryBaseline, baselineOK, baselineErr := s.recoveryNodeBaselineForHead(scanHead)
	if baselineErr != nil {
		return false, baselineErr
	}
	if !baselineOK {
		return false, nil
	}
	missingBaseline, missingBaselineSet := original.recoveryNodeBaseline()
	if !missingBaselineSet {
		missingBaseline = 0
		missingBaselineSet = true
	} else if missingBaseline > recoveryBaseline {
		// C clamps the copied stack head before it forks the missing version.
		missingBaseline = recoveryBaseline
	}
	recoveryGroup := s.nextSeq

	// Replace the sole head temporarily. This lets the existing S3 primitive
	// build the absorb lineage without duplicating its fail-closed checks.
	absorbHeader := original
	absorbHeader.head = scanHead
	absorbHeader.paused = false
	absorbHeader.shifted = false
	missingHeader := absorbHeader
	missingHeader.creationSeq = s.nextSeq
	replacements := [...]diagnosticParserCoreHeader{absorbHeader, missingHeader}
	s.invalidateVerifierHeaderBinding()
	s.headers = replaceDiagnosticParserCoreHeader(s.headers, index, replacements[:])
	restore := func() {
		s.invalidateVerifierHeaderBinding()
		clear(s.headers)
		s.headers = s.headers[:1]
		s.headers[0] = original
	}

	absorbed, absorbErr := s.s3TryOpenErrorRegionWithAlternatives(index, true)
	if absorbErr != nil {
		restore()
		return false, absorbErr
	}
	if !absorbed || s.headers[index].recoveryRegion() == nil || !s.headers[index].shifted {
		restore()
		return false, nil
	}

	missingHead, insertErr := s.compact.ShiftMissingLeaf(scanHead, targetState, core.Symbol(missing), byteOffset)
	if insertErr != nil {
		restore()
		return false, insertErr
	}
	s.headers[index+1].head = missingHead
	s.headers[index].publishRecoveryCondenseState(recoveryGroup, 0, recoveryBaseline, true)
	s.headers[index+1].publishRecoveryCondenseState(
		0, recoveryGroup, missingBaseline, missingBaselineSet,
	)
	s.headers[index].markRecoveryLineage()
	s.headers[index+1].markRecoveryLineage()
	s.recoveryIsolation = true
	s.nextSeq++
	s.s5MissingInsertions++
	return true, nil
}
