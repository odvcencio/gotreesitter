//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Tree-sitter C retains at most six distinct live version keys after recovery
// condensation. Histories with one retained key remain separate here because
// the compact graph represents C's merged links as separate headers.
const diagnosticParserCoreRecoveryVersionLimit = 6

type diagnosticParserCoreRecoveryCondenseKey struct {
	state         core.StateID
	byteOffset    uint32
	cost          uint32
	checkpoint    core.CheckpointID
	recoveryGroup uint64
	missingGroup  uint64
	shifted       bool
	paused        bool
}

type diagnosticParserCoreRecoveryCondenseEntry struct {
	header diagnosticParserCoreHeader
	status core.RecoveryErrorStatus
	key    diagnosticParserCoreRecoveryCondenseKey
}

func diagnosticParserCoreMergeEquivalentRecoveryStatus(
	target *diagnosticParserCoreRecoveryCondenseEntry,
	candidate diagnosticParserCoreRecoveryCondenseEntry,
) {
	if target == nil {
		return
	}
	if candidate.status.NodeCount > target.status.NodeCount {
		target.status.NodeCount = candidate.status.NodeCount
	}
	if candidate.status.DynPrec > target.status.DynPrec {
		target.status.DynPrec = candidate.status.DynPrec
	}
}

func (s *diagnosticParserCoreGenericScheduler) mergeEquivalentRecoveryCondenseEntriesOwned(
	owner core.SchedulerTransactionToken,
	target *diagnosticParserCoreRecoveryCondenseEntry,
	candidate diagnosticParserCoreRecoveryCondenseEntry,
	symbols []core.SelectedSymbolPolicy,
	source core.RecoveryCostSource,
	memo *core.RecoveryCostMemo,
) (bool, error) {
	return s.mergeEquivalentRecoveryEntriesOwned(owner, target, candidate, symbols, source, memo, false)
}

func (s *diagnosticParserCoreGenericScheduler) mergeEquivalentRecoveryEntriesOwned(
	owner core.SchedulerTransactionToken,
	target *diagnosticParserCoreRecoveryCondenseEntry,
	candidate diagnosticParserCoreRecoveryCondenseEntry,
	symbols []core.SelectedSymbolPolicy,
	source core.RecoveryCostSource,
	memo *core.RecoveryCostMemo,
	preserveSibling bool,
) (bool, error) {
	if s == nil || s.compact == nil || target == nil || target.header.accepted ||
		candidate.header.accepted || target.header.paused || candidate.header.paused ||
		target.header.recoveryRegion() != nil || candidate.header.recoveryRegion() != nil ||
		target.key != candidate.key ||
		!s.versionLexerStateEqual(target.header.versionState, candidate.header.versionState) {
		return false, nil
	}
	incumbent := target.header.head
	incoming := candidate.header.head
	canonical, ok := s.compact.CanonicalBoundary(
		target.key.state,
		target.key.byteOffset,
		target.key.shifted,
		target.key.checkpoint,
	)
	if !ok {
		return false, nil
	}
	if canonical == incoming && !preserveSibling {
		incumbent, incoming = incoming, incumbent
	} else if canonical != incumbent && canonical != incoming {
		return false, nil
	}
	payloadHasError := func(payload core.SubtreeID) (bool, error) {
		cost, err := core.RecoveryNodeErrorCostMemo(symbols, source, memo, payload)
		return cost > 0, err
	}
	mergeHeads := s.compact.MergeEquivalentRecoveryHeadsOwned
	if preserveSibling {
		mergeHeads = s.compact.MergeEquivalentRecoverySiblingHeadsOwned
	}
	merged, err := mergeHeads(
		owner,
		target.key.state,
		target.key.byteOffset,
		target.key.checkpoint,
		target.key.shifted,
		incumbent,
		incoming,
		payloadHasError,
	)
	if err != nil {
		return false, err
	}
	target.header.head = merged
	target.header.recoveryFlags |= candidate.header.recoveryFlags
	target.header.frontierSequence = mergeDiagnosticParserCoreFrontier(
		target.header.frontierSequence,
		candidate.header.frontierSequence,
	)
	target.header.cleanPathRank, target.header.cleanPathLineage = mergeDiagnosticParserCoreCleanPathLineage(
		target.header.cleanPathRank,
		target.header.cleanPathLineage,
		candidate.header.cleanPathRank,
		candidate.header.cleanPathLineage,
	)
	target.header.convergedReductionSplit = target.header.convergedReductionSplit || candidate.header.convergedReductionSplit
	target.header.resurrectionUnproved = target.header.resurrectionUnproved || candidate.header.resurrectionUnproved
	if candidate.header.altSet.Len() != 0 {
		incomparable := s.compact.AlternativeSetIncomparable(target.header.altSet, candidate.header.altSet)
		s.compact.UnionAlternativeSet(&target.header.altSet, candidate.header.altSet)
		target.header.blended = target.header.blended || candidate.header.blended || incomparable
	}
	if !candidate.header.dropCohortRefs.Empty() || candidate.header.dropCohortRefs.Overflowed() || candidate.header.dropCohortRefs.Blended() {
		if _, err := s.compact.UnionDropCohortRefsChecked(
			&target.header.dropCohortRefs,
			candidate.header.dropCohortRefs,
		); err != nil {
			return false, err
		}
	}
	diagnosticParserCoreMergeEquivalentRecoveryStatus(target, candidate)
	s.invalidateVerifierHeaderBinding()
	return true, nil
}

func diagnosticParserCoreRecoveryCondenseSameGroup(
	left, right diagnosticParserCoreRecoveryCondenseEntry,
) bool {
	return left.key.recoveryGroup != 0 && left.key.recoveryGroup == right.key.recoveryGroup
}

func diagnosticParserCoreRecoveryCondenseShouldStayBefore(
	left, right diagnosticParserCoreRecoveryCondenseEntry,
) bool {
	if left.header.paused || right.header.paused {
		return false
	}
	return left.key.recoveryGroup != 0 && right.key.recoveryGroup == 0 &&
		right.key.missingGroup == left.key.recoveryGroup
}

func diagnosticParserCoreRecoveryCondensePairwise(
	entries []diagnosticParserCoreRecoveryCondenseEntry,
	order []int,
) []int {
	return diagnosticParserCoreRecoveryCondensePairwiseMode(entries, order, false)
}

func diagnosticParserCoreRecoveryCondensePairwiseMode(
	entries []diagnosticParserCoreRecoveryCondenseEntry,
	order []int,
	ownedTurns bool,
) []int {
	for i := 1; i < len(order); i++ {
		removedCurrent := false
		for j := 0; j < i; j++ {
			left := entries[order[j]]
			right := entries[order[i]]
			if diagnosticParserCoreRecoveryCondenseSameGroup(left, right) ||
				(!ownedTurns && diagnosticParserCoreRecoveryCondenseShouldStayBefore(left, right)) {
				continue
			}
			if !ownedTurns && diagnosticParserCoreRecoveryCondenseShouldStayBefore(right, left) {
				order[i], order[j] = order[j], order[i]
				continue
			}
			switch core.RecoveryCompareVersions(left.status, right.status) {
			case core.RecoveryComparisonTakeLeft:
				order = append(order[:i], order[i+1:]...)
				i--
				removedCurrent = true
			case core.RecoveryComparisonPreferLeft, core.RecoveryComparisonNone:
			case core.RecoveryComparisonPreferRight:
				order[i], order[j] = order[j], order[i]
			case core.RecoveryComparisonTakeRight:
				order = append(order[:j], order[j+1:]...)
				i--
				j--
			}
			if removedCurrent || i < 1 {
				break
			}
		}
	}
	return order
}

func diagnosticParserCoreDerivationVisibleNodeCount(
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	derivation core.Derivation,
) (uint32, error) {
	var total uint32
	for _, payload := range derivation.Payloads {
		count, err := src.compact.CachedVisibleSubtreeCount(symbols, payload)
		if err != nil {
			return 0, err
		}
		if math.MaxUint32-total < count {
			return 0, errors.New("parser-core phase zero: recovery visible-node count overflow")
		}
		total += count
	}
	return total, nil
}

func diagnosticParserCoreOpenRegionVisibleNodeCount(
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	region *diagnosticParserCoreS3Region,
) (uint32, error) {
	if region == nil || len(region.children) == 0 {
		return 0, nil
	}
	total := uint32(1)
	for _, child := range region.children {
		count, err := src.compact.CachedVisibleSubtreeCount(symbols, child)
		if err != nil {
			return 0, err
		}
		if math.MaxUint32-total < count {
			return 0, errors.New("parser-core phase zero: recovery region visible-node count overflow")
		}
		total += count
	}
	return total, nil
}

func diagnosticParserCoreRecoveryCumulativeVisibleNodeCount(
	derivation core.Derivation,
	region *diagnosticParserCoreS3Region,
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
) (uint32, error) {
	count, err := diagnosticParserCoreDerivationVisibleNodeCount(symbols, src, derivation)
	if err != nil {
		return 0, err
	}
	regionCount, err := diagnosticParserCoreOpenRegionVisibleNodeCount(symbols, src, region)
	if err != nil {
		return 0, err
	}
	if math.MaxUint32-count < regionCount {
		return 0, errors.New("parser-core phase zero: recovery cumulative visible-node count overflow")
	}
	return count + regionCount, nil
}

func (s *diagnosticParserCoreGenericScheduler) recoveryCumulativeVisibleNodeCount(
	head core.Head,
	region *diagnosticParserCoreS3Region,
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
) (uint32, bool, error) {
	derivations, err := s.compact.Derivations(head)
	if err != nil {
		if errors.Is(err, core.ErrDerivationEnumerationCap) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if len(derivations) != 1 {
		return 0, false, nil
	}
	count, err := diagnosticParserCoreRecoveryCumulativeVisibleNodeCount(
		derivations[0], region, symbols, src,
	)
	return count, true, err
}

func (s *diagnosticParserCoreGenericScheduler) recoveryNodeBaselineForHead(
	head core.Head,
) (uint32, bool, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return 0, false, nil
	}
	src, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
	if err != nil {
		return 0, false, err
	}
	return s.recoveryCumulativeVisibleNodeCount(
		head, nil, diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language), src,
	)
}

// resetRecoveryNodeBaseline records C's current cumulative count after a
// pause or ERROR-state entry. The caller decides whether an unsupported
// ambiguous graph must decline the compact route.
func (s *diagnosticParserCoreGenericScheduler) resetRecoveryNodeBaseline(
	header *diagnosticParserCoreHeader,
) (bool, error) {
	if header == nil {
		return false, nil
	}
	baseline, supported, err := s.recoveryNodeBaselineForHead(header.head)
	if err != nil || !supported {
		return supported, err
	}
	header.publishRecoveryCondenseState(
		header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(),
		baseline, true,
	)
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) recoveryCondenseEntry(
	header diagnosticParserCoreHeader,
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	memo *core.RecoveryCostMemo,
) (diagnosticParserCoreRecoveryCondenseEntry, bool, error) {
	// Recovery heads can contain several physical paths after the S5 marker
	// merge. Price the graph aggregate directly, instead of enumerating paths.
	// The aggregate rejects unequal path costs and reports the maximum visible
	// count needed by C's node-count tie band.
	aggregate, supported, err := s.compact.RecoveryGraphAggregateForHead(header.head, symbols, src)
	if err != nil {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, err
	}
	if !supported {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, nil
	}
	stackCost := aggregate.MinimumErrorCost
	currentCount := aggregate.MaximumVisibleCount
	state, byteOffset, err := s.compact.Boundary(header.head)
	if err != nil {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, err
	}
	region := header.recoveryRegion()
	if region != nil {
		state = 0
		byteOffset = region.endByte
		if len(region.children) != 0 {
			regionCost, regionErr := core.RecoveryErrorRegionCost(
				symbols, src, memo,
				region.startByte, src.rowAt(region.startByte),
				region.endByte, src.rowAt(region.endByte),
				region.children,
			)
			if regionErr != nil {
				return diagnosticParserCoreRecoveryCondenseEntry{}, false, regionErr
			}
			if math.MaxUint32-stackCost < regionCost {
				return diagnosticParserCoreRecoveryCondenseEntry{}, false, errors.New("parser-core phase zero: recovery condense cost overflow")
			}
			stackCost += regionCost
			regionCount, countErr := diagnosticParserCoreOpenRegionVisibleNodeCount(symbols, src, region)
			if countErr != nil {
				return diagnosticParserCoreRecoveryCondenseEntry{}, false, countErr
			}
			if math.MaxUint32-currentCount < regionCount {
				return diagnosticParserCoreRecoveryCondenseEntry{}, false, errors.New("parser-core phase zero: recovery condense visible-node count overflow")
			}
			currentCount += regionCount
		}
	}
	openSegments := header.recoveryOpenSegments()
	if openSegments < 0 {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, errors.New("parser-core phase zero: negative recovery segment count")
	}
	openCost := uint64(openSegments) * uint64(core.RecoveryCostPerRecovery)
	if openCost > math.MaxUint32 || stackCost > math.MaxUint32-uint32(openCost) {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, errors.New("parser-core phase zero: recovery condense open-segment cost overflow")
	}
	stackCost += uint32(openCost)
	baseline, baselineSet := header.recoveryNodeBaseline()
	if !baselineSet {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, nil
	}
	if baseline > currentCount {
		baseline = currentCount
		header.publishRecoveryCondenseState(
			header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(),
			baseline, true,
		)
	}
	nodeCount := currentCount - baseline
	if uint64(nodeCount) > uint64(math.MaxInt) {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, nil
	}
	if aggregate.StoredPrecedenceMaximum > int64(math.MaxInt) || aggregate.StoredPrecedenceMaximum < -int64(math.MaxInt)-1 {
		return diagnosticParserCoreRecoveryCondenseEntry{}, false, nil
	}
	return diagnosticParserCoreRecoveryCondenseEntry{
		header: header,
		status: core.RecoveryVersionStatus(
			stackCost, header.paused, int(nodeCount), int(aggregate.StoredPrecedenceMaximum), region != nil,
		),
		key: diagnosticParserCoreRecoveryCondenseKey{
			state: state, byteOffset: byteOffset, cost: stackCost,
			checkpoint:    header.checkpoint,
			recoveryGroup: header.recoveryGroupIdentity(),
			missingGroup:  header.recoveryMissingGroupIdentity(),
			shifted:       header.shifted, paused: header.paused,
		},
	}, true, nil
}

// condenseRecoveryVersions applies C's pairwise recovery competition before
// it keeps the first six distinct merge keys. Accepted versions stay outside
// the competition and retain their acceptance order.
func (s *diagnosticParserCoreGenericScheduler) condenseRecoveryVersions(
	owner core.SchedulerTransactionToken,
	headers []diagnosticParserCoreHeader,
) ([]diagnosticParserCoreHeader, uint64, bool, error) {
	if s == nil || s.compact == nil || !s.recoveryIsolation ||
		!s.options.allowCompactRecoveryLineageSelection || len(headers) < 2 {
		return headers, 0, false, nil
	}
	activeCount := 0
	for index := range headers {
		if !headers[index].isRecoveryLineage() {
			return headers, 0, false, nil
		}
		if !headers[index].accepted {
			activeCount++
		}
	}
	if activeCount == 0 {
		return headers, 0, false, nil
	}
	if s.tokenSource == nil || s.tokenSource.language == nil || len(s.options.materializationSource) == 0 {
		return headers, 0, false, nil
	}
	src, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
	if err != nil {
		return headers, 0, false, nil
	}
	scratch := s.recoveryCondenseScratch[:0]
	if cap(scratch) < len(headers) {
		scratch = make([]diagnosticParserCoreRecoveryCondenseEntry, 0, len(headers))
	}
	order := s.recoveryCondenseOrderScratch[:0]
	if cap(order) < activeCount {
		order = make([]int, 0, activeCount)
	}
	clearScratch := func() {
		clear(scratch)
		s.recoveryCondenseScratch = scratch[:0]
		clear(order)
		s.recoveryCondenseOrderScratch = order[:0]
	}
	var memo core.RecoveryCostMemo
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	for index := range headers {
		if headers[index].accepted {
			continue
		}
		entry, supported, entryErr := s.recoveryCondenseEntry(headers[index], symbols, src, &memo)
		if entryErr != nil {
			clearScratch()
			return headers, 0, false, entryErr
		}
		if !supported {
			clearScratch()
			return headers, 0, false, nil
		}
		representative := -1
		for prior := range scratch {
			if scratch[prior].key == entry.key {
				representative = prior
				break
			}
		}
		if representative < 0 {
			scratch = append(scratch, entry)
			order = append(order, len(scratch)-1)
			continue
		}
		merged, mergeErr := s.mergeEquivalentRecoveryCondenseEntriesOwned(
			owner,
			&scratch[representative],
			entry,
			symbols,
			src,
			&memo,
		)
		if mergeErr != nil {
			clearScratch()
			return headers, 0, false, mergeErr
		}
		if !merged {
			scratch = append(scratch, entry)
			diagnosticParserCoreMergeEquivalentRecoveryStatus(&scratch[representative], entry)
		}
	}
	activeEntries := len(scratch)
	for index := range headers {
		if headers[index].accepted {
			scratch = append(scratch, diagnosticParserCoreRecoveryCondenseEntry{header: headers[index]})
		}
	}
	order = diagnosticParserCoreRecoveryCondensePairwiseMode(scratch, order, s.recoveryTurns.active)

	var drops uint64
	if len(order) > diagnosticParserCoreRecoveryVersionLimit {
		for _, droppedRepresentative := range order[diagnosticParserCoreRecoveryVersionLimit:] {
			key := scratch[droppedRepresentative].key
			for index := 0; index < activeEntries; index++ {
				if scratch[index].key == key {
					drops++
				}
			}
		}
		order = order[:diagnosticParserCoreRecoveryVersionLimit]
	}

	write := 0
	for _, representative := range order {
		key := scratch[representative].key
		for index := 0; index < activeEntries; index++ {
			if scratch[index].key != key {
				continue
			}
			headers[write] = scratch[index].header
			write++
		}
	}
	for index := activeEntries; index < len(scratch); index++ {
		headers[write] = scratch[index].header
		write++
	}
	clearDiagnosticParserCoreHeaderSuffix(headers, write)
	headers = headers[:write]
	clearScratch()
	return headers, drops, true, nil
}
