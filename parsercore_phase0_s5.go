//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// diagnosticParserCoreS5ReductionMode selects the C recovery reduction scan.
// AnyTerminal excludes EOF. ExactToken scans only the supplied symbol.
type diagnosticParserCoreS5ReductionMode uint8

const (
	diagnosticParserCoreS5AnyTerminal diagnosticParserCoreS5ReductionMode = iota
	diagnosticParserCoreS5ExactToken
)

type diagnosticParserCoreS5ReductionAction struct {
	symbol  core.Symbol
	value   core.Action
	ordinal int
}

type diagnosticParserCoreS5ReductionKey struct {
	symbol core.Symbol
	count  uint8
}

type diagnosticParserCoreS5ReductionMetadata struct {
	rank                    core.CleanPathRankSelection
	lineage                 uint16
	set                     core.AlternativeSet
	setBlended              bool
	convergedReductionSplit bool
	resurrectionUnproved    bool
	dropCohortRefs          core.DropCohortRefSet
}

// s5MissingLineageNeedsOwnedLexer identifies the C condense-tail shape in
// which the absorber consumed the shared token and the missing sibling must
// request its next token from its own byte and parser state.
func s5MissingLineageNeedsOwnedLexer(
	s *diagnosticParserCoreGenericScheduler,
	indices []int,
) bool {
	if s == nil || !s.recoveryIsolation || s.s5MissingInsertions == 0 ||
		len(s.headers) != 2 || len(indices) != 1 {
		return false
	}
	missingIndex := indices[0]
	if missingIndex < 0 || missingIndex >= len(s.headers) {
		return false
	}
	missing := &s.headers[missingIndex]
	missingGroup := missing.recoveryMissingGroupIdentity()
	if missingGroup == 0 || missing.recoveryRegion() != nil || missing.paused ||
		!missing.isRecoveryLineage() {
		return false
	}
	for index := range s.headers {
		if index == missingIndex {
			continue
		}
		absorber := &s.headers[index]
		if absorber.shifted && absorber.isRecoveryLineage() &&
			absorber.recoveryGroupIdentity() == missingGroup {
			return true
		}
	}
	return false
}

// diagnosticParserCoreS5SchedulerSnapshot protects scheduler state that Core
// does not include in its transaction checkpoint.
type diagnosticParserCoreS5SchedulerSnapshot struct {
	value                diagnosticParserCoreGenericScheduler
	receipt              DiagnosticParserCoreGenericScheduler
	receiptPointer       *DiagnosticParserCoreGenericScheduler
	receiptPresent       bool
	receiptIsBacking     bool
	tokenSourceSnapshot  dfaRelexSnapshot
	tokenSourceState     StateID
	tokenSourceGLRStates []StateID
	tokenSourcePresent   bool
}

func diagnosticParserCoreS5CloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	copy(out, src)
	return out
}

func captureDiagnosticParserCoreS5Scheduler(s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreS5SchedulerSnapshot {
	if s == nil {
		return diagnosticParserCoreS5SchedulerSnapshot{}
	}
	value := *s
	value.headers = diagnosticParserCoreS5CloneSlice(s.headers)
	value.versionLexerRequests = diagnosticParserCoreS5CloneSlice(s.versionLexerRequests)
	value.reductionOutputs = diagnosticParserCoreS5CloneSlice(s.reductionOutputs)
	value.reductionReplacements = diagnosticParserCoreS5CloneSlice(s.reductionReplacements)
	value.recoveryCondenseScratch = diagnosticParserCoreS5CloneSlice(s.recoveryCondenseScratch)
	value.recoveryCondenseOrderScratch = diagnosticParserCoreS5CloneSlice(s.recoveryCondenseOrderScratch)
	value.classifiedBoundaries = diagnosticParserCoreS5CloneSlice(s.classifiedBoundaries)
	value.condenseCandidates = diagnosticParserCoreS5CloneSlice(s.condenseCandidates)
	value.electStates = diagnosticParserCoreS5CloneSlice(s.electStates)
	value.electGLRStates = diagnosticParserCoreS5CloneSlice(s.electGLRStates)
	value.acceptedPayloads = diagnosticParserCoreS5CloneSlice(s.acceptedPayloads)
	value.summaryHeaderScratch = diagnosticParserCoreS5CloneSlice(s.summaryHeaderScratch)
	value.dispatchScratch.cells = diagnosticParserCoreS5CloneSlice(s.dispatchScratch.cells)
	value.dispatchScratch.noActionIndices = diagnosticParserCoreS5CloneSlice(s.dispatchScratch.noActionIndices)
	value.conflictScratch.actionOutputs = diagnosticParserCoreS5CloneSlice(s.conflictScratch.actionOutputs)
	value.conflictScratch.reductionOutputs = diagnosticParserCoreS5CloneSlice(s.conflictScratch.reductionOutputs)
	value.conflictScratch.outputs = diagnosticParserCoreS5CloneSlice(s.conflictScratch.outputs)
	value.conflictScratch.armRanges = diagnosticParserCoreS5CloneSlice(s.conflictScratch.armRanges)
	value.conflictScratch.adopted = diagnosticParserCoreS5CloneSlice(s.conflictScratch.adopted)
	value.conflictScratch.headerAssembly = diagnosticParserCoreS5CloneSlice(s.conflictScratch.headerAssembly)
	value.headerRollbackScratch.headers = diagnosticParserCoreS5CloneSlice(s.headerRollbackScratch.headers)
	value.footprintRefs = diagnosticParserCoreS5CloneSlice(s.footprintRefs)
	value.canonicalScratch.keys = diagnosticParserCoreS5CloneSlice(s.canonicalScratch.keys)
	value.canonicalScratch.headerBuffers[0] = diagnosticParserCoreS5CloneSlice(s.canonicalScratch.headerBuffers[0])
	value.canonicalScratch.headerBuffers[1] = diagnosticParserCoreS5CloneSlice(s.canonicalScratch.headerBuffers[1])
	if s.canonicalScratch.groups != nil {
		value.canonicalScratch.groups = make(map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup, len(s.canonicalScratch.groups))
		for key, group := range s.canonicalScratch.groups {
			value.canonicalScratch.groups[key] = group
		}
	}
	if s.receipt != nil {
		value.receipt = nil
	}
	snapshot := diagnosticParserCoreS5SchedulerSnapshot{value: value}
	if s.receipt != nil {
		snapshot.receipt = *s.receipt
		snapshot.receiptPointer = s.receipt
		snapshot.receiptPresent = true
		snapshot.receiptIsBacking = s.receipt == &s.receiptBacking
		snapshot.receipt.StartHeaders = diagnosticParserCoreS5CloneSlice(s.receipt.StartHeaders)
		snapshot.receipt.Rounds = diagnosticParserCoreS5CloneSlice(s.receipt.Rounds)
		snapshot.receipt.Conflicts = diagnosticParserCoreS5CloneSlice(s.receipt.Conflicts)
		snapshot.receipt.ExternalShifts = diagnosticParserCoreS5CloneSlice(s.receipt.ExternalShifts)
		snapshot.receipt.Elections = diagnosticParserCoreS5CloneSlice(s.receipt.Elections)
		snapshot.receipt.VersionLexerRequests = diagnosticParserCoreS5CloneSlice(s.receipt.VersionLexerRequests)
		snapshot.receipt.NoActionDrops = diagnosticParserCoreS5CloneSlice(s.receipt.NoActionDrops)
	}
	if s.tokenSource != nil && s.tokenSource.lexer != nil {
		snapshot.tokenSourcePresent = true
		snapshot.tokenSourceSnapshot = cloneDiagnosticParserCoreDFARelexSnapshot(s.tokenSource.snapshotRelexState())
		snapshot.tokenSourceState = s.tokenSource.state
		snapshot.tokenSourceGLRStates = diagnosticParserCoreS5CloneSlice(s.tokenSource.glrStates)
	}
	return snapshot
}

func (snapshot diagnosticParserCoreS5SchedulerSnapshot) restore(s *diagnosticParserCoreGenericScheduler) {
	if s == nil {
		return
	}
	value := snapshot.value
	*s = value
	if snapshot.receiptPresent {
		if snapshot.receiptIsBacking {
			s.receiptBacking = snapshot.receipt
			s.receipt = &s.receiptBacking
		} else if snapshot.receiptPointer != nil {
			*snapshot.receiptPointer = snapshot.receipt
			s.receipt = snapshot.receiptPointer
		}
	}
	s.verifierHeaderPtr = nil
	s.verifierBound = 0
	if snapshot.tokenSourcePresent && s.tokenSource != nil && s.tokenSource.lexer != nil {
		snapshot.tokenSourceSnapshot.restore(s.tokenSource)
		s.tokenSource.state = snapshot.tokenSourceState
		s.tokenSource.glrStates = diagnosticParserCoreS5CloneSlice(snapshot.tokenSourceGLRStates)
	}
}

func (s *diagnosticParserCoreGenericScheduler) s5RecoverySource() (*diagnosticParserCoreRecoveryCostSource, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil {
		return nil, errors.New("parser-core phase zero: S5 recovery source is unavailable")
	}
	source := s.options.materializationSource
	if len(source) == 0 && s.tokenSource.lexer != nil {
		// Direct scheduler fixtures bind only the lexer source. Production
		// materialization keeps its authenticated source in options.
		source = s.tokenSource.lexer.source
	}
	return newDiagnosticParserCoreRecoveryCostSource(s.compact, source)
}

func (s *diagnosticParserCoreGenericScheduler) s5RecoveryOutputCostFunc() (core.ReductionOutputCostFunc, *core.RecoveryCostMemo, error) {
	source, err := s.s5RecoverySource()
	if err != nil {
		return nil, nil, err
	}
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return nil, nil, errors.New("parser-core phase zero: S5 recovery language is unavailable")
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	memo := new(core.RecoveryCostMemo)
	cost := func(prev core.NodeID, payload core.SubtreeID) (uint32, error) {
		prefix, err := s.compact.RecoveryStoredErrorCost(core.Head{Node: prev})
		if err != nil {
			return 0, err
		}
		payloadCost, err := core.RecoveryNodeErrorCostMemo(symbols, source, memo, payload)
		if err != nil {
			return 0, err
		}
		if math.MaxUint32-prefix < payloadCost {
			return 0, errors.New("parser-core phase zero: S5 recovery output cost overflow")
		}
		return prefix + payloadCost, nil
	}
	return cost, memo, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5CollectReductionActions(
	state core.StateID,
	mode diagnosticParserCoreS5ReductionMode,
	lookahead core.Symbol,
) ([]diagnosticParserCoreS5ReductionAction, bool, error) {
	if s == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return nil, false, errors.New("parser-core phase zero: S5 action table is unavailable")
	}
	var symbols []core.Symbol
	if mode == diagnosticParserCoreS5ExactToken {
		symbols = []core.Symbol{lookahead}
	} else {
		tokenCount := core.Symbol(s.tokenSource.language.TokenCount)
		if tokenCount <= 1 {
			return nil, false, nil
		}
		symbols = make([]core.Symbol, 0, int(tokenCount)-1)
		for symbol := core.Symbol(1); symbol < tokenCount; symbol++ {
			symbols = append(symbols, symbol)
		}
	}
	seen := make(map[diagnosticParserCoreS5ReductionKey]struct{})
	actions := make([]diagnosticParserCoreS5ReductionAction, 0, 4)
	hasShift := false
	for symbolIndex, symbol := range symbols {
		if symbolIndex&63 == 0 {
			if err := s.pollStopControl(); err != nil {
				return nil, false, err
			}
		}
		row, err := s.compact.Actions(state, symbol)
		if err != nil {
			return nil, false, err
		}
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			action := row.At(ordinal)
			switch action.Type {
			case core.ActionShift:
				if !action.Extra && !action.Repetition {
					hasShift = true
				}
			case core.ActionRecover:
				if !action.Extra && !action.Repetition {
					hasShift = true
				}
			case core.ActionAccept:
				hasShift = true
			case core.ActionReduce:
				if action.ChildCount == 0 {
					continue
				}
				key := diagnosticParserCoreS5ReductionKey{symbol: action.Symbol, count: action.ChildCount}
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				actions = append(actions, diagnosticParserCoreS5ReductionAction{
					symbol: symbol, value: action, ordinal: ordinal,
				})
			}
		}
	}
	return actions, hasShift, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5CondenseCandidates(
	start, end, source int,
) []core.CondenseCandidate {
	out := make([]core.CondenseCandidate, 0, max(end-start, 0))
	for index := start; index < end && index < len(s.headers); index++ {
		if index == source {
			continue
		}
		header := s.headers[index]
		if header.accepted || header.paused || header.recoveryRegion() != nil ||
			header.isRecoveryLineage() || header.isRecoveryCosted() {
			continue
		}
		storedCost, err := s.compact.RecoveryStoredErrorCost(header.head)
		if err != nil {
			// The caller cannot return an error from this scratch builder. Leave
			// malformed or unavailable heads out of the candidate scope; the
			// reduction itself authenticates its source and reports any failure.
			continue
		}
		out = append(out, core.CondenseCandidate{
			Head: header.head, Checkpoint: header.checkpoint,
			DropCohortRefs: header.dropCohortRefs, ErrorCost: storedCost,
			MergeIdentity: s.condenseCandidateMergeIdentity(index), Shifted: header.shifted,
		})
	}
	return out
}

// s5MergeReductionVersionOwned applies C's pre-classification merge only to
// the versions created by this frontier call. The incumbent keeps its slot.
func (s *diagnosticParserCoreGenericScheduler) s5MergeReductionVersionOwned(
	owner core.SchedulerTransactionToken,
	incumbentIndex, incomingIndex int,
) (bool, error) {
	if incumbentIndex < 0 || incomingIndex < 0 || incumbentIndex >= incomingIndex || incomingIndex >= len(s.headers) {
		return false, errors.New("parser-core phase zero: S5 reduction merge indices are invalid")
	}
	incumbent := &s.headers[incumbentIndex]
	incoming := &s.headers[incomingIndex]
	if incumbent.accepted || incoming.accepted || incumbent.paused || incoming.paused ||
		incumbent.recoveryRegion() != nil || incoming.recoveryRegion() != nil ||
		incumbent.isRecoveryLineage() || incoming.isRecoveryLineage() ||
		incumbent.isRecoveryCosted() || incoming.isRecoveryCosted() {
		return false, nil
	}
	if !s.versionLexerStateEqual(incumbent.versionState, incoming.versionState) ||
		incumbent.checkpoint != incoming.checkpoint || incumbent.shifted != incoming.shifted {
		return false, nil
	}
	incumbentState, incumbentByte, err := s.compact.Boundary(incumbent.head)
	if err != nil {
		return false, err
	}
	incomingState, incomingByte, err := s.compact.Boundary(incoming.head)
	if err != nil {
		return false, err
	}
	if incumbentState != incomingState || incumbentByte != incomingByte {
		return false, nil
	}
	incumbentCost, err := s.compact.RecoveryStoredErrorCost(incumbent.head)
	if err != nil {
		return false, err
	}
	incomingCost, err := s.compact.RecoveryStoredErrorCost(incoming.head)
	if err != nil {
		return false, err
	}
	if incumbentCost != incomingCost {
		return false, nil
	}
	merged, err := s.compact.MergeEquivalentHeadsWithStoredErrorCostOwned(
		owner, incumbentState, incumbentByte, incumbent.checkpoint,
		incumbent.shifted, incumbent.head, incoming.head,
	)
	if err != nil {
		return false, err
	}
	incumbent.head = merged
	incumbent.frontierSequence = mergeDiagnosticParserCoreFrontier(
		incumbent.frontierSequence, incoming.frontierSequence,
	)
	incumbent.cleanPathRank, incumbent.cleanPathLineage = mergeDiagnosticParserCoreCleanPathLineage(
		incumbent.cleanPathRank, incumbent.cleanPathLineage,
		incoming.cleanPathRank, incoming.cleanPathLineage,
	)
	incumbent.convergedReductionSplit = incumbent.convergedReductionSplit || incoming.convergedReductionSplit
	incumbent.resurrectionUnproved = incumbent.resurrectionUnproved || incoming.resurrectionUnproved
	if incoming.altSet.Len() != 0 {
		incomparable := s.compact.AlternativeSetIncomparable(incumbent.altSet, incoming.altSet)
		s.compact.UnionAlternativeSet(&incumbent.altSet, incoming.altSet)
		incumbent.blended = incumbent.blended || incoming.blended || incomparable
	}
	if !incoming.dropCohortRefs.Empty() || incoming.dropCohortRefs.Overflowed() || incoming.dropCohortRefs.Blended() {
		if _, err := s.compact.UnionDropCohortRefsChecked(&incumbent.dropCohortRefs, incoming.dropCohortRefs); err != nil {
			return false, err
		}
	}
	s.invalidateVerifierHeaderBinding()
	copy(s.headers[incomingIndex:], s.headers[incomingIndex+1:])
	clear(s.headers[len(s.headers)-1:])
	s.headers = s.headers[:len(s.headers)-1]
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5ReductionOutputMetadata(
	output core.ReductionOutput,
	outputIndex int,
	lineage uint16,
) diagnosticParserCoreS5ReductionMetadata {
	metadata := diagnosticParserCoreS5ReductionMetadata{
		rank:    output.CleanPathRank,
		lineage: lineage,
		convergedReductionSplit: output.MultiplePopPaths ||
			output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged,
		resurrectionUnproved: output.HistoricalBoundaryProvenance == core.HistoricalBoundaryUnproved,
		dropCohortRefs:       output.DropCohortRefs,
	}
	if output.MultiplePopPaths {
		metadata.set = core.NewAlternativeSetMember(lineage, uint16(outputIndex))
	} else if output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged {
		metadata.rank = output.HistoricalCleanPathRank
		metadata.lineage = output.HistoricalCleanPathLineage
	}
	if output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged &&
		output.HistoricalAlternativeSet.Len() != 0 {
		incomparable := s.compact.AlternativeSetIncomparable(metadata.set, output.HistoricalAlternativeSet)
		s.compact.UnionAlternativeSet(&metadata.set, output.HistoricalAlternativeSet)
		metadata.setBlended = output.HistoricalBlended || incomparable
	}
	return metadata
}

func (s *diagnosticParserCoreGenericScheduler) s5ApplyReductionOutputMetadata(
	header *diagnosticParserCoreHeader,
	output core.ReductionOutput,
	metadata diagnosticParserCoreS5ReductionMetadata,
) error {
	if header == nil {
		return errors.New("parser-core phase zero: nil S5 reduction header")
	}
	header.head = output.Head
	header.paused = false
	header.accepted = false
	header.shifted = false
	header.freshness = output.Freshness
	header.convergedReductionSplit = header.convergedReductionSplit || metadata.convergedReductionSplit
	header.resurrectionUnproved = header.resurrectionUnproved || metadata.resurrectionUnproved
	applyDiagnosticParserCoreCleanPathOutput(header, metadata.rank, metadata.lineage)
	if metadata.set.Len() != 0 {
		s.compact.UnionAlternativeSet(&header.altSet, metadata.set)
		header.blended = header.blended || metadata.setBlended
	}
	if !metadata.dropCohortRefs.Empty() || metadata.dropCohortRefs.Overflowed() || metadata.dropCohortRefs.Blended() {
		if _, err := s.compact.UnionDropCohortRefsChecked(&header.dropCohortRefs, metadata.dropCohortRefs); err != nil {
			return err
		}
	}
	return nil
}

// s5UpdatedReductionSiblingIndex mirrors the ordinary adoption selector. The
// S5 caller retains the exact destination so it never assigns freshness to a
// different same-boundary header after adoption.
func (s *diagnosticParserCoreGenericScheduler) s5UpdatedReductionSiblingIndex(
	source int,
	head core.Head,
) (int, error) {
	if s == nil || source < 0 || source >= len(s.headers) {
		return -1, errors.New("parser-core phase zero: S5 sibling adoption source is out of range")
	}
	sourceVersionState := s.headers[source].versionState
	for index := range s.headers {
		if index == source {
			continue
		}
		if s.recoveryIsolation &&
			(s.headers[source].isRecoveryLineage() || s.headers[index].isRecoveryLineage()) {
			continue
		}
		header := s.headers[index]
		if !s.versionLexerStateEqual(header.versionState, sourceVersionState) {
			continue
		}
		state, byteOffset, err := s.compact.Boundary(header.head)
		if err != nil {
			return -1, err
		}
		canonical, ok := s.compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint)
		if ok && canonical == head {
			return index, nil
		}
	}
	return -1, nil
}

// s5RunReductionFrontierOwned ports C's do_all_potential_reductions for the
// compact scheduler. The caller owns the surrounding scheduler speculation.
func (s *diagnosticParserCoreGenericScheduler) s5RunReductionFrontierOwned(
	owner core.SchedulerTransactionToken,
	start int,
	mode diagnosticParserCoreS5ReductionMode,
	lookahead core.Symbol,
	staged *diagnosticParserCoreS5Work,
) (canShift bool, err error) {
	if s == nil || staged == nil || start < 0 || start >= len(s.headers) {
		return false, errors.New("parser-core phase zero: S5 reduction frontier is incomplete")
	}
	initialVersionCount := len(s.headers)
	startingVersion := start
	version := start
	iteration := uint64(0)
	for version < len(s.headers) {
		if iteration == math.MaxUint64 {
			return false, errors.New("parser-core phase zero: S5 reduction iteration overflow")
		}
		promotionIndex := iteration
		iteration++
		prePassVersionCount := len(s.headers)
		if version < 0 || version >= prePassVersionCount {
			break
		}
		// C merges the current version with only versions created by this
		// frontier call before it reads the current action row. Never fold
		// against the original scheduler suffix here.
		merged := false
		for prior := initialVersionCount; prior < version; prior++ {
			if prior >= len(s.headers) {
				break
			}
			merged, err = s.s5MergeReductionVersionOwned(owner, prior, version)
			if err != nil {
				return false, err
			}
			if merged {
				break
			}
		}
		if merged {
			continue
		}
		header := s.headers[version]
		state, _, err := s.compact.Boundary(header.head)
		if err != nil {
			return false, err
		}
		actions, hasShift, err := s.s5CollectReductionActions(state, mode, lookahead)
		if err != nil {
			return false, err
		}
		lastReduction := -1
		for _, reduction := range actions {
			staged.potentialReductionActions++
			// C can merge a new reduction output into any lower live version,
			// except the reduction source. Capture that frontier before this
			// action can append more versions.
			preActionVersionCount := len(s.headers)
			boundary, err := s.compact.ClassifyBoundary(header.head, reduction.symbol)
			if err != nil {
				return false, err
			}
			candidates := s.s5CondenseCandidates(0, preActionVersionCount, version)
			storedCost, err := s.compact.RecoveryStoredErrorCost(header.head)
			if err != nil {
				return false, err
			}
			var cost core.ReductionOutputCostFunc
			var memo *core.RecoveryCostMemo
			if storedCost != 0 || header.recoveryRegion() != nil || header.isRecoveryCosted() || header.isRecoveryLineage() {
				cost, memo, err = s.s5RecoveryOutputCostFunc()
				if err != nil {
					return false, err
				}
			}
			var outputs []core.ReductionOutput
			s.compact.SetReduceConflictContext(true)
			func() {
				defer s.compact.SetReduceConflictContext(false)
				if cost == nil {
					outputs, err = s.compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(
						owner, candidates, s.reductionOutputs, boundary, reduction.ordinal, core.ForkOrder{},
					)
				} else {
					outputs, err = s.compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
						owner, candidates, s.reductionOutputs, boundary, reduction.ordinal, core.ForkOrder{}, cost,
					)
				}
			}()
			if memo != nil {
				memo.Reset()
			}
			if err != nil {
				return false, err
			}
			staged.potentialReductionOutputs += uint64(len(outputs))
			actionReductionVersion := -1
			var lineage uint16
			if len(outputs) != 0 && outputs[0].MultiplePopPaths {
				lineage, err = nextDiagnosticParserCoreCleanPathLineage(&s.nextCleanPathLineage)
				if err != nil {
					return false, err
				}
				if err := s.compact.RecordReductionLineageOwned(owner, outputs, lineage); err != nil {
					return false, err
				}
			}
			for outputIndex, output := range outputs {
				if output.Freshness != core.ReductionUnchanged && output.Freshness != core.ReductionUpdated && output.Freshness != core.ReductionNew {
					return false, errors.New("parser-core phase zero: S5 reduction returned invalid freshness")
				}
				metadata := s.s5ReductionOutputMetadata(output, outputIndex, lineage)
				if output.Freshness == core.ReductionUnchanged || output.Freshness == core.ReductionUpdated {
					slot, slotErr := s.s5UpdatedReductionSiblingIndex(version, output.Head)
					if slotErr != nil {
						return false, slotErr
					}
					if slot < 0 {
						return false, errors.New("parser-core phase zero: S5 reduction output lost its scheduler version")
					}
					adopted, adoptErr := s.adoptUpdatedReductionSiblingOwned(
						owner, version, output.Head,
						metadata.rank, metadata.lineage, metadata.set, metadata.setBlended,
						metadata.dropCohortRefs, metadata.convergedReductionSplit,
						metadata.resurrectionUnproved, core.DropCohortProducerSiblingAdoption,
					)
					if adoptErr != nil {
						return false, adoptErr
					}
					if !adopted {
						return false, errors.New("parser-core phase zero: S5 reduction output lost its scheduler version")
					}
					s.headers[slot].freshness = output.Freshness
					// C reports STACK_VERSION_NONE for an output already
					// represented by a live version. It must never become a
					// promotion candidate merely because its metadata changed.
					continue
				}
				if output.Freshness != core.ReductionNew {
					continue
				}
				replacement := header
				if err := s.s5ApplyReductionOutputMetadata(&replacement, output, metadata); err != nil {
					return false, err
				}
				if s.nextSeq == math.MaxUint64 {
					return false, errors.New("parser-core phase zero: S5 reduction creation sequence overflow")
				}
				replacement.creationSeq = s.nextSeq
				s.nextSeq++
				s.headers = append(s.headers, replacement)
				if actionReductionVersion < 0 {
					actionReductionVersion = len(s.headers) - 1
				}
			}
			// C overwrites reduction_version after each action, including an
			// action that produced no new physical version.
			lastReduction = actionReductionVersion
			header = s.headers[version]
		}
		if hasShift {
			canShift = true
		}
		if hasShift {
			if version == startingVersion {
				// The original suffix has already been classified by the
				// scheduler. Continue with only outputs appended by this pass.
				version = prePassVersionCount
			} else {
				version++
			}
			continue
		}
		if lastReduction >= 0 && promotionIndex < 6 {
			promoted := s.headers[lastReduction]
			if lastReduction <= version {
				return false, errors.New("parser-core phase zero: S5 reduction promotion order is invalid")
			}
			s.headers[version] = promoted
			s.invalidateVerifierHeaderBinding()
			copy(s.headers[lastReduction:], s.headers[lastReduction+1:])
			clear(s.headers[len(s.headers)-1:])
			s.headers = s.headers[:len(s.headers)-1]
			staged.reductionPromotions++
			continue
		}
		if mode == diagnosticParserCoreS5ExactToken {
			generatedStart := prePassVersionCount
			copy(s.headers[version:], s.headers[version+1:])
			clear(s.headers[len(s.headers)-1:])
			s.headers = s.headers[:len(s.headers)-1]
			if version < generatedStart {
				generatedStart--
			}
			next := version + 1
			if next < generatedStart {
				next = generatedStart
			}
			version = next
			continue
		}
		if version == startingVersion {
			version = prePassVersionCount
		} else {
			version++
		}
	}
	return canShift, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5RecoveryBaseline(headers []diagnosticParserCoreHeader) (uint32, bool, error) {
	source, err := s.s5RecoverySource()
	if err != nil {
		return 0, false, err
	}
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return 0, false, nil
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	var maximum uint32
	for _, header := range headers {
		aggregate, supported, err := s.compact.RecoveryGraphAggregateForHead(header.head, symbols, source)
		if err != nil {
			return 0, false, err
		}
		if !supported {
			return 0, false, nil
		}
		if aggregate.MaximumVisibleCount > maximum {
			maximum = aggregate.MaximumVisibleCount
		}
	}
	return maximum, true, nil
}

// s5MissingLeafDependency reproduces the lexer reset and mark-end sequence
// that C uses before it publishes a missing leaf. The compact boundary stores
// the parser position before padding, so included-range normalization must run
// against a copy of the lexer cursor and must not mutate the live token source.
func s5MissingLeafDependency(lexer *Lexer, source []byte, stackByte, lookaheadEndByte uint32) (core.MissingLeafDependency, error) {
	if uint64(stackByte) > uint64(len(source)) || lookaheadEndByte < stackByte {
		return core.MissingLeafDependency{}, errors.New("parser-core phase zero: S5 missing dependency is outside source")
	}
	stackPoint := advancePointByBytes(Point{}, source[:stackByte])
	positionedByte := stackByte
	positionedPoint := stackPoint
	if lexer != nil && len(lexer.includedRanges) != 0 {
		if logicalPoint, ok := lexer.includedPointAtPosition(int(stackByte)); ok {
			stackPoint = logicalPoint
		}
		cursor := includedLexerCursor{
			pos:      int(stackByte),
			row:      stackPoint.Row,
			col:      stackPoint.Column,
			rangeIdx: lexer.includedRangeIndexForPosition(int(stackByte)),
		}
		lexer.normalizeIncludedCursor(&cursor)
		positionedByte = uint32(cursor.pos)
		positionedPoint = Point{Row: cursor.row, Column: cursor.col}
	}

	var paddingBytes uint32
	if positionedByte >= stackByte {
		paddingBytes = positionedByte - stackByte
	}
	paddingExtent := Point{}
	if extent, ok := pointExtentBetween(stackPoint, positionedPoint); ok && positionedByte >= stackByte {
		paddingExtent = extent
	}
	return core.MissingLeafDependency{
		StackByte:      stackByte,
		StackRow:       stackPoint.Row,
		StackColumn:    stackPoint.Column,
		PaddingBytes:   paddingBytes,
		PaddingRows:    paddingExtent.Row,
		PaddingColumn:  paddingExtent.Column,
		LookaheadBytes: lookaheadEndByte - stackByte,
	}, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5ShiftMissingLeafOwned(
	owner core.SchedulerTransactionToken,
	headerIndex int,
	targetState core.StateID,
	symbol core.Symbol,
	byteOffset uint32,
) error {
	if headerIndex < 0 || headerIndex >= len(s.headers) {
		return errors.New("parser-core phase zero: S5 missing header index is out of range")
	}
	source := s.options.materializationSource
	if source == nil && s.tokenSource != nil && s.tokenSource.lexer != nil {
		source = s.tokenSource.lexer.source
	}
	lookaheadEndByte := tokenLookaheadEndByte(s.token)
	lexer := (*Lexer)(nil)
	if s.tokenSource != nil {
		lexer = s.tokenSource.lexer
	}
	dependency, err := s5MissingLeafDependency(lexer, source, byteOffset, lookaheadEndByte)
	if err != nil {
		return err
	}
	head, err := s.compact.ShiftMissingLeafWithDependencyOwned(owner, s.headers[headerIndex].head, targetState, symbol, dependency)
	if err != nil {
		return err
	}
	s.headers[headerIndex].head = head
	s.headers[headerIndex].shifted = false
	s.headers[headerIndex].paused = false
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) s5TryMissingCandidateOwned(
	owner core.SchedulerTransactionToken,
	anyHeaders []diagnosticParserCoreHeader,
	anyIndex int,
	missing core.Symbol,
	targetState core.StateID,
	byteOffset uint32,
	staged *diagnosticParserCoreS5Work,
) ([]diagnosticParserCoreHeader, bool, uint64, error) {
	if anyIndex < 0 || anyIndex >= len(anyHeaders) {
		return nil, false, 0, errors.New("parser-core phase zero: S5 missing candidate index is out of range")
	}
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	stagedBefore := *staged
	var trialHeaders []diagnosticParserCoreHeader
	var trialSeq uint64
	viable := false
	err := s.compact.ApplySchedulerSpeculation(owner, func(trialOwner core.SchedulerTransactionToken) (bool, error) {
		s.headers = []diagnosticParserCoreHeader{anyHeaders[anyIndex]}
		if s.nextSeq == math.MaxUint64 {
			return false, errors.New("parser-core phase zero: S5 missing creation sequence overflow")
		}
		trialSeq = s.nextSeq
		s.headers[0].creationSeq = trialSeq
		s.nextSeq++
		if err := s.s5ShiftMissingLeafOwned(trialOwner, 0, targetState, missing, byteOffset); err != nil {
			return false, err
		}
		mode := diagnosticParserCoreS5ExactToken
		if s.token.Symbol == 0 {
			// Native tree-sitter treats symbol zero as the any-lookahead
			// sentinel during the EOF missing-token trial.
			mode = diagnosticParserCoreS5AnyTerminal
		}
		canShift, err := s.s5RunReductionFrontierOwned(trialOwner, 0, mode, core.Symbol(s.token.Symbol), staged)
		if err != nil {
			return false, err
		}
		if !canShift || len(s.headers) == 0 {
			return false, nil
		}
		trialHeaders = diagnosticParserCoreS5CloneSlice(s.headers)
		viable = true
		return true, nil
	})
	if err != nil {
		snapshot.restore(s)
		*staged = stagedBefore
		return nil, false, 0, err
	}
	if !viable {
		snapshot.restore(s)
		*staged = stagedBefore
		return nil, false, 0, nil
	}
	return trialHeaders, true, trialSeq, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5AppendAndMergeAbsorberOwned(
	owner core.SchedulerTransactionToken,
	anyHeaders []diagnosticParserCoreHeader,
	baseline uint32,
	recoveryGroup uint64,
	staged *diagnosticParserCoreS5Work,
) (diagnosticParserCoreHeader, error) {
	if len(anyHeaders) == 0 {
		return diagnosticParserCoreHeader{}, errors.New("parser-core phase zero: S5 absorber has no reduction heads")
	}
	state, byteOffset, err := s.compact.Boundary(anyHeaders[0].head)
	if err != nil {
		return diagnosticParserCoreHeader{}, err
	}
	if byteOffset > s.token.StartByte {
		return diagnosticParserCoreHeader{}, errors.New("parser-core phase zero: S5 absorber boundary passes the token start")
	}
	checkpoint := anyHeaders[0].checkpoint
	storedCost, err := s.compact.RecoveryStoredErrorCost(anyHeaders[0].head)
	if err != nil {
		return diagnosticParserCoreHeader{}, err
	}
	context := core.RecoveryDiscontinuityContext{ErrorState: 0, ByteOffset: byteOffset, Checkpoint: checkpoint}
	for _, header := range anyHeaders {
		_, candidateByte, err := s.compact.Boundary(header.head)
		if err != nil {
			return diagnosticParserCoreHeader{}, err
		}
		if candidateByte != byteOffset || header.checkpoint != checkpoint {
			return diagnosticParserCoreHeader{}, errors.New("parser-core phase zero: S5 absorber heads do not share a boundary")
		}
		candidateCost, err := s.compact.RecoveryStoredErrorCost(header.head)
		if err != nil {
			return diagnosticParserCoreHeader{}, err
		}
		if candidateCost != storedCost {
			return diagnosticParserCoreHeader{}, nil
		}
		if !s.versionLexerStateEqual(anyHeaders[0].versionState, header.versionState) {
			return diagnosticParserCoreHeader{}, nil
		}
	}
	markers := make([]core.Head, 0, len(anyHeaders))
	for _, header := range anyHeaders {
		marker, err := s.compact.AppendRecoveryDiscontinuityOwned(owner, header.head, context)
		if err != nil {
			return diagnosticParserCoreHeader{}, err
		}
		markers = append(markers, marker)
	}
	incumbent := markers[0]
	for index := 1; index < len(markers); index++ {
		merged, err := s.compact.MergeRecoveryDiscontinuityHeadsOwned(owner, context, incumbent, markers[index])
		if err != nil {
			return diagnosticParserCoreHeader{}, err
		}
		incumbent = merged
		staged.recoveryDiscontinuityMerges++
	}
	tokenExtra, err := s3TokenIsExtraShift(s.compact, s.token.Symbol)
	if err != nil {
		return diagnosticParserCoreHeader{}, err
	}
	leaf, err := s.compact.ErrorRegionLeaf(core.Symbol(s.token.Symbol), s.token.StartByte, s.token.EndByte, tokenExtra)
	if err != nil {
		return diagnosticParserCoreHeader{}, err
	}
	absorb := anyHeaders[0]
	absorb.head = incumbent
	absorb.paused = false
	absorb.shifted = true
	absorb.accepted = false
	absorb.openRecoveryRegion(&diagnosticParserCoreS3Region{
		state: state, startByte: s.token.StartByte, endByte: s.token.EndByte,
		children: []core.SubtreeID{leaf},
	})
	absorb.publishRecoveryCondenseState(recoveryGroup, 0, baseline, true)
	absorb.markRecoveryCosted()
	for _, header := range anyHeaders[1:] {
		absorb.frontierSequence = mergeDiagnosticParserCoreFrontier(
			absorb.frontierSequence, header.frontierSequence,
		)
		absorb.cleanPathRank, absorb.cleanPathLineage = mergeDiagnosticParserCoreCleanPathLineage(
			absorb.cleanPathRank, absorb.cleanPathLineage,
			header.cleanPathRank, header.cleanPathLineage,
		)
		absorb.convergedReductionSplit = absorb.convergedReductionSplit || header.convergedReductionSplit
		absorb.resurrectionUnproved = absorb.resurrectionUnproved || header.resurrectionUnproved
		if _, err := s.compact.UnionDropCohortRefsChecked(&absorb.dropCohortRefs, header.dropCohortRefs); err != nil {
			return diagnosticParserCoreHeader{}, err
		}
		s.compact.UnionAlternativeSet(&absorb.altSet, header.altSet)
		absorb.blended = absorb.blended || header.blended
	}
	s.s3RegionOpened = true
	return absorb, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5RunOwned(
	owner core.SchedulerTransactionToken,
	index int,
	staged *diagnosticParserCoreS5Work,
) (bool, error) {
	if !s.s5MissingTokenAdmitted() || len(s.headers) != 1 || index != 0 ||
		s.s5MissingInsertions >= maxDiagnosticParserCoreMissingInsertions {
		return false, nil
	}
	if s.token.Missing || s.token.NoLookahead || s.token.Symbol == errorSymbol ||
		s.tokenSource == nil || s.tokenSource.language == nil || s.headers[index].recoveryRegion() != nil {
		return false, nil
	}
	if s.token.Symbol == 0 && !s.options.allowCompactS5EOFMissingInsertion {
		return false, nil
	}
	tokenCount := core.Symbol(s.tokenSource.language.TokenCount)
	if tokenCount <= 1 || tokenCount > math.MaxUint16 {
		return false, nil
	}
	original := s.headers[index]
	_, originalByte, err := s.compact.Boundary(original.head)
	if err != nil {
		return false, err
	}
	if originalByte > s.token.StartByte {
		return false, nil
	}
	if _, err := s.compact.RecoveryStoredErrorCost(original.head); err != nil {
		return false, err
	}
	if _, err := s.s5RunReductionFrontierOwned(owner, index, diagnosticParserCoreS5AnyTerminal, core.Symbol(s.token.Symbol), staged); err != nil {
		return false, err
	}
	anyHeaders := diagnosticParserCoreS5CloneSlice(s.headers)
	if len(anyHeaders) == 0 {
		return false, nil
	}
	var missingHeaders []diagnosticParserCoreHeader
	var missingGroup uint64
	for anyIndex := range anyHeaders {
		state, byteOffset, err := s.compact.Boundary(anyHeaders[anyIndex].head)
		if err != nil {
			return false, err
		}
		if byteOffset > s.token.StartByte {
			continue
		}
		for rawSymbol := core.Symbol(1); rawSymbol < tokenCount; rawSymbol++ {
			if uint32(rawSymbol)&63 == 0 {
				if err := s.pollStopControl(); err != nil {
					return false, err
				}
			}
			row, err := s.compact.Actions(state, rawSymbol)
			if err != nil {
				return false, err
			}
			if row.Len() == 0 {
				continue
			}
			last := row.At(row.Len() - 1)
			if last.Type != core.ActionShift {
				continue
			}
			nextState := last.State
			if last.Extra {
				nextState = state
			}
			if nextState == 0 || nextState == state {
				continue
			}
			nextRow, err := s.compact.Actions(nextState, core.Symbol(s.token.Symbol))
			if err != nil {
				return false, err
			}
			// C's ts_language_has_reduce_action checks only the leading action.
			// A later reduce arm does not admit a missing-token trial.
			if nextRow.Len() == 0 || nextRow.At(0).Type != core.ActionReduce {
				continue
			}
			staged.missingTokenTrials++
			trial, viable, seq, err := s.s5TryMissingCandidateOwned(owner, anyHeaders, anyIndex, rawSymbol, nextState, originalByte, staged)
			if err != nil {
				return false, err
			}
			if !viable {
				continue
			}
			missingHeaders = trial
			missingGroup = seq
			break
		}
		if len(missingHeaders) != 0 {
			break
		}
	}
	if len(missingHeaders) == 0 {
		return false, nil
	}
	if s.token.Symbol == 0 {
		missingBaseline, missingBaselineSet := original.recoveryNodeBaseline()
		if !missingBaselineSet {
			missingBaselineSet = true
		}
		for index := range missingHeaders {
			missingHeaders[index].publishRecoveryCondenseState(0, missingGroup, missingBaseline, missingBaselineSet)
			missingHeaders[index].paused = false
			missingHeaders[index].shifted = false
			missingHeaders[index].markRecoveryCosted()
			missingHeaders[index].markRecoveryLineage()
		}
		s.invalidateVerifierHeaderBinding()
		clear(s.headers)
		s.headers = append(s.headers[:0], missingHeaders...)
		s.recoveryIsolation = true
		s.epochProgress = true
		s.s5MissingInsertions++
		staged.missingTokenCommits++
		if err := s.canonicalizeOwned(owner); err != nil {
			return false, err
		}
		if err := s.persistHeaderLineageOwned(owner); err != nil {
			return false, err
		}
		if uint64(len(s.headers)) > s.work.PeakLiveVersions {
			s.work.PeakLiveVersions = uint64(len(s.headers))
		}
		return true, nil
	}
	baseline, baselineOK, err := s.s5RecoveryBaseline(anyHeaders)
	if err != nil {
		return false, err
	}
	if !baselineOK {
		return false, nil
	}
	s.headers = anyHeaders
	absorb, err := s.s5AppendAndMergeAbsorberOwned(owner, anyHeaders, baseline, missingGroup, staged)
	if err != nil {
		return false, err
	}
	if absorb.head.Node == 0 {
		return false, nil
	}
	missingBaseline, missingBaselineSet := original.recoveryNodeBaseline()
	if !missingBaselineSet {
		missingBaseline = 0
		missingBaselineSet = true
	} else if missingBaseline > baseline {
		missingBaseline = baseline
	}
	for index := range missingHeaders {
		missingHeaders[index].publishRecoveryCondenseState(0, missingGroup, missingBaseline, missingBaselineSet)
		missingHeaders[index].paused = false
		missingHeaders[index].shifted = false
		missingHeaders[index].markRecoveryCosted()
	}
	absorb.publishRecoveryCondenseState(missingGroup, 0, baseline, true)
	absorb.markRecoveryLineage()
	for index := range missingHeaders {
		missingHeaders[index].markRecoveryLineage()
	}
	replacements := make([]diagnosticParserCoreHeader, 1, 1+len(missingHeaders))
	replacements[0] = absorb
	replacements = append(replacements, missingHeaders...)
	s.invalidateVerifierHeaderBinding()
	// The AnyTerminal frontier can contain several generated reductions.
	// Replace every old slot so unmarked dead versions cannot cross the
	// recovery competition boundary.
	clear(s.headers)
	s.headers = s.headers[:0]
	s.headers = append(s.headers, replacements...)
	s.recoveryIsolation = true
	s.epochProgress = true
	s.s5MissingInsertions++
	staged.missingTokenCommits++
	if s.options.allowCompactRecoveryVersionTurns {
		s.recoveryTurns = diagnosticParserCoreRecoveryTurns{active: true, lastByte: originalByte}
	}
	// Match ordinary scheduler publication. Canonicalization clears stale
	// freshness and reconciles equivalent physical heads before ownership is
	// persisted for the next dispatch.
	if err := s.canonicalizeOwned(owner); err != nil {
		return false, err
	}
	if err := s.persistHeaderLineageOwned(owner); err != nil {
		return false, err
	}
	if uint64(len(s.headers)) > s.work.PeakLiveVersions {
		s.work.PeakLiveVersions = uint64(len(s.headers))
	}
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) s5TryMissingTokenInsertionFaithful(index int) (handled bool, err error) {
	// Keep unsupported shapes on the cheap path. The full snapshot is reserved
	// for an admitted sole header that can enter the S5 transaction.
	if s == nil || s.compact == nil || !s.s5MissingTokenAdmitted() ||
		len(s.headers) != 1 || index != 0 ||
		s.s5MissingInsertions >= maxDiagnosticParserCoreMissingInsertions ||
		s.token.Missing || s.token.NoLookahead || s.token.Symbol == errorSymbol ||
		s.tokenSource == nil || s.tokenSource.language == nil ||
		s.headers[0].recoveryRegion() != nil {
		return false, nil
	}
	if s.token.Symbol == 0 && !s.options.allowCompactS5EOFMissingInsertion {
		return false, nil
	}
	// This route owns the first recovery episode. Earlier shared recovery or
	// sibling drops require advance ordering that this route has not executed.
	if s.options.allowCompactRecoveryVersionTurns &&
		(s.s3RegionOpened || s.s3ResumeCount != 0 || s.work.NoActionDrops != 0) {
		return false, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRecovery,
			detail:   "owned recovery requires no prior shared recovery or no-action drops",
		}
	}
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	staged := diagnosticParserCoreS5Work{}
	restored := false
	restore := func() {
		if restored {
			return
		}
		snapshot.restore(s)
		restored = true
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			restore()
			panic(recovered)
		}
		if err != nil || !handled {
			restore()
		}
	}()
	run := func(parent core.SchedulerTransactionToken) error {
		var committed bool
		if err := s.compact.ApplySchedulerSpeculation(parent, func(owner core.SchedulerTransactionToken) (bool, error) {
			var runErr error
			committed, runErr = s.s5RunOwned(owner, index, &staged)
			return committed, runErr
		}); err != nil {
			return err
		}
		if !committed {
			return nil
		}
		s.commitS5Work(staged)
		handled = true
		return nil
	}
	if s.freshSessionOwner != nil {
		err = run(*s.freshSessionOwner)
	} else {
		err = s.compact.ApplySchedulerAtomic(run)
	}
	if err != nil {
		handled = false
	}
	return handled, err
}
