//go:build gts_eof_recovery_shadow

package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"unsafe"
)

const (
	// DiagnosticEOFRecoveryMaxPayloads bounds one private recover_eof fold.
	DiagnosticEOFRecoveryMaxPayloads = 4096
	// DiagnosticEOFRecoveryMaxCloneBytes bounds the detached Core through
	// materialization, its append reserve, and its isolated provider wrapper.
	DiagnosticEOFRecoveryMaxCloneBytes = 16 << 20
)

// DiagnosticEOFRecoveryForkReceipt proves one bounded private fold. The
// equality fields cover only copied headers, arenas, and work.
type DiagnosticEOFRecoveryForkReceipt struct {
	Steps                uint32
	MaxSteps             uint32
	Payloads             uint32
	MaxPayloads          uint32
	SourceFootprintBytes uint64
	CoreHeaderBytes      uint64
	CopiedArenaBytes     uint64
	AppendReserveBytes   uint64
	MapBytes             uint64
	TemporaryBytes       uint64
	PreservationBytes    uint64
	ProviderWrapperBytes uint64
	// PeakCloneBytes covers all detached Core storage through materialization,
	// its append reserve, and the isolated reduction-plan wrapper. It excludes
	// the Parser, runner scratch, and materialized tree.
	PeakCloneBytes                uint64
	MaxCloneBytes                 uint64
	StartByte                     uint32
	EndByte                       uint32
	SubtreesBefore                uint32
	SubtreesAfter                 uint32
	ChildrenBefore                uint32
	ChildrenAfter                 uint32
	CheckpointMapEntries          uint32
	RetainedSelectedPolicy        bool
	SourceSchedulerActive         bool
	SchedulerFrameDetached        bool
	SourceProvidersDetached       bool
	ValidationStructuralPositions uint32
	ValidationRemappedFields      uint32
	ValidationRemappedAliases     uint32
	ValidationScratchReserved     bool
	CopiedArenaPrefixesEqual      bool
	CopiedHeadersEqual            bool
	// MetadataConstructionUnauthenticated proves that the diagnostic ERROR
	// publication cleared the grammar-table authentication marker.
	MetadataConstructionUnauthenticated bool
	RootChildrenExact                   bool
	MutableStorageDisjoint              bool
	WorkBefore                          Work
	WorkAfter                           Work
}

type diagnosticEOFRecoveryClonePlan struct {
	coreHeaderBytes      uint64
	copiedArenaBytes     uint64
	appendReserveBytes   uint64
	mapBytes             uint64
	temporaryBytes       uint64
	preservationBytes    uint64
	providerWrapperBytes uint64
	validationDemand     diagnosticEOFRecoveryValidationDemand
	peakBytes            uint64
}

type diagnosticEOFRecoveryValidationDemand struct {
	structuralPositions int
	remappedFields      int
	remappedAliases     int
}

// DiagnosticEOFRecoveryValidationScratch reports the three Core-owned
// validation backings that can grow during generic materialization.
type DiagnosticEOFRecoveryValidationScratch struct {
	StructuralPositions uint32
	RemappedFields      uint32
	RemappedAliases     uint32
}

// DiagnosticEOFRecoveryProviderReceipt proves that one detached shadow uses
// only a distinct, read-only reduction-plan provider.
type DiagnosticEOFRecoveryProviderReceipt struct {
	SourceProvidersDetached   bool
	ReductionPlansAttached    bool
	ProviderDiffersFromSource bool
	TableViewDetached         bool
	SelectedStoreDetached     bool
}

// ForkDiagnosticEOFRecovery copies the stable compact arenas and appends one
// private ERROR root. It does not retain a live parser or provider pointer.
func ForkDiagnosticEOFRecovery(
	live *Core,
	head Head,
	payloads []SubtreeID,
	providerWrapperBytes uint64,
) (*Core, SubtreeID, DiagnosticEOFRecoveryForkReceipt, error) {
	receipt := DiagnosticEOFRecoveryForkReceipt{
		MaxSteps:      1,
		MaxPayloads:   DiagnosticEOFRecoveryMaxPayloads,
		MaxCloneBytes: DiagnosticEOFRecoveryMaxCloneBytes,
	}
	if live == nil {
		return nil, 0, receipt, errors.New("parser-core phase zero: nil EOF recovery source")
	}
	if _, err := live.node(head.Node); err != nil {
		return nil, 0, receipt, err
	}
	if len(payloads) == 0 || len(payloads) > DiagnosticEOFRecoveryMaxPayloads {
		return nil, 0, receipt, fmt.Errorf(
			"parser-core phase zero: EOF recovery payload count %d is outside 1..%d",
			len(payloads), DiagnosticEOFRecoveryMaxPayloads,
		)
	}

	receipt.Payloads = uint32(len(payloads))
	receipt.SourceFootprintBytes = live.FootprintBytes()
	receipt.CheckpointMapEntries = uint32(len(live.checkpoints.buckets))
	receipt.RetainedSelectedPolicy = live.selectedPolicy != nil
	receipt.SourceSchedulerActive = live.schedulerFrame.active
	plan, err := planDiagnosticEOFRecoveryClone(live, payloads, providerWrapperBytes)
	receipt.CoreHeaderBytes = plan.coreHeaderBytes
	receipt.CopiedArenaBytes = plan.copiedArenaBytes
	receipt.AppendReserveBytes = plan.appendReserveBytes
	receipt.MapBytes = plan.mapBytes
	receipt.TemporaryBytes = plan.temporaryBytes
	receipt.PreservationBytes = plan.preservationBytes
	receipt.ProviderWrapperBytes = plan.providerWrapperBytes
	receipt.ValidationStructuralPositions = uint32(plan.validationDemand.structuralPositions)
	receipt.ValidationRemappedFields = uint32(plan.validationDemand.remappedFields)
	receipt.ValidationRemappedAliases = uint32(plan.validationDemand.remappedAliases)
	receipt.PeakCloneBytes = plan.peakBytes
	if err != nil {
		return nil, 0, receipt, err
	}

	shadow := cloneDiagnosticEOFRecoveryCore(live, len(payloads), plan.validationDemand)
	receipt.SchedulerFrameDetached = !shadow.schedulerFrame.active && len(shadow.transactions) == 0
	receipt.SourceProvidersDetached = shadow.tables == nil && shadow.plans == nil && shadow.selectedProvider == nil
	receipt.ValidationScratchReserved = shadow.DiagnosticEOFRecoveryValidationScratch() == (DiagnosticEOFRecoveryValidationScratch{
		StructuralPositions: receipt.ValidationStructuralPositions,
		RemappedFields:      receipt.ValidationRemappedFields,
		RemappedAliases:     receipt.ValidationRemappedAliases,
	})
	receipt.MutableStorageDisjoint = diagnosticEOFRecoveryStorageDisjoint(live, shadow)
	receipt.CopiedHeadersEqual = diagnosticEOFRecoveryCopiedHeadersEqual(live, shadow)
	if !receipt.MutableStorageDisjoint {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery clone aliases copied storage")
	}
	if !receipt.ValidationScratchReserved {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery validation scratch reserve mismatch")
	}

	first, err := shadow.subtree(payloads[0])
	if err != nil {
		return nil, 0, receipt, err
	}
	last, err := shadow.subtree(payloads[len(payloads)-1])
	if err != nil {
		return nil, 0, receipt, err
	}
	if last.endByte < first.startByte {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery payload span is reversed")
	}

	receipt.StartByte = first.startByte
	receipt.EndByte = last.endByte
	receipt.SubtreesBefore = uint32(len(shadow.subtrees))
	receipt.ChildrenBefore = uint32(len(shadow.children))
	receipt.WorkBefore = shadow.Work()
	root, err := shadow.appendSubtree(subtreeRecord{
		symbol: ErrorRegionSymbol, startByte: first.startByte, endByte: last.endByte,
	}, payloads, nil, nil)
	receipt.MetadataConstructionUnauthenticated = !shadow.metadataConstructionAuthenticated
	if err != nil {
		return nil, 0, receipt, err
	}
	shadow.eofRecoveryRoots = append(shadow.eofRecoveryRoots, root)
	receipt.Steps = 1
	receipt.SubtreesAfter = uint32(len(shadow.subtrees))
	receipt.ChildrenAfter = uint32(len(shadow.children))
	receipt.CopiedArenaPrefixesEqual = diagnosticEOFRecoveryCopiedArenasEqual(live, shadow)
	receipt.RootChildrenExact = diagnosticEOFRecoveryRootChildrenEqual(shadow, root, payloads)
	receipt.WorkAfter = shadow.Work()
	return shadow, root, receipt, nil
}

// BindDiagnosticEOFRecoveryReductionPlans attaches one narrow provider to a
// detached shadow. It rejects all wider capabilities and all source aliases.
func (c *Core) BindDiagnosticEOFRecoveryReductionPlans(
	source *Core,
	provider ReductionPlanProvider,
) (DiagnosticEOFRecoveryProviderReceipt, error) {
	receipt := DiagnosticEOFRecoveryProviderReceipt{}
	if c == nil || source == nil {
		return receipt, errors.New("parser-core phase zero: EOF recovery provider bind requires two cores")
	}
	if c == source {
		return receipt, errors.New("parser-core phase zero: EOF recovery provider bind requires a detached shadow")
	}
	receipt.SourceProvidersDetached = c.tables == nil && c.plans == nil && c.selectedProvider == nil
	if !receipt.SourceProvidersDetached {
		return receipt, errors.New("parser-core phase zero: EOF recovery shadow has a prebound provider")
	}
	if provider == nil {
		return receipt, errors.New("parser-core phase zero: EOF recovery reduction-plan provider is nil")
	}
	if _, ok := provider.(TableView); ok {
		return receipt, errors.New("parser-core phase zero: EOF recovery provider exposes a table view")
	}
	if _, ok := provider.(SelectedStorePolicyProvider); ok {
		return receipt, errors.New("parser-core phase zero: EOF recovery provider exposes selected-store capability")
	}
	if !diagnosticEOFRecoveryPointerProvider(provider) {
		return receipt, errors.New("parser-core phase zero: EOF recovery provider is not a pointer-owned wrapper")
	}
	for _, sourceProvider := range []any{source.tables, source.plans, source.selectedProvider} {
		if diagnosticEOFRecoveryProvidersAlias(provider, sourceProvider) {
			return receipt, errors.New("parser-core phase zero: EOF recovery provider aliases the source provider")
		}
	}
	c.plans = provider
	receipt.ReductionPlansAttached = c.plans != nil
	receipt.ProviderDiffersFromSource = true
	receipt.TableViewDetached = c.tables == nil
	receipt.SelectedStoreDetached = c.selectedProvider == nil
	return receipt, nil
}

func diagnosticEOFRecoveryPointerProvider(provider any) bool {
	value := reflect.ValueOf(provider)
	return value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil()
}

func diagnosticEOFRecoveryProvidersAlias(left, right any) bool {
	if !diagnosticEOFRecoveryPointerProvider(left) || !diagnosticEOFRecoveryPointerProvider(right) ||
		reflect.TypeOf(left) != reflect.TypeOf(right) {
		return false
	}
	return reflect.ValueOf(left).Pointer() == reflect.ValueOf(right).Pointer()
}

func planDiagnosticEOFRecoveryClone(live *Core, payloads []SubtreeID, providerWrapperBytes uint64) (diagnosticEOFRecoveryClonePlan, error) {
	plan := diagnosticEOFRecoveryClonePlan{
		coreHeaderBytes:      uint64(unsafe.Sizeof(Core{})),
		providerWrapperBytes: providerWrapperBytes,
	}
	if providerWrapperBytes == 0 {
		return plan, errors.New("parser-core phase zero: EOF recovery provider wrapper storage is not accounted")
	}
	if live.selectedPolicy != nil {
		return plan, errors.New("parser-core phase zero: EOF recovery clone does not support a retained selected policy")
	}
	if len(live.checkpoints.buckets) != 0 {
		return plan, errors.New("parser-core phase zero: EOF recovery clone does not support a nonempty checkpoint map")
	}
	if len(live.transactions) != 0 || live.condenseScopeActive {
		return plan, errors.New("parser-core phase zero: EOF recovery source has active transactional state")
	}
	if !live.metadataConstructionAuthenticated {
		return plan, errors.New("parser-core phase zero: EOF recovery source metadata is not authenticated")
	}
	if live.plans == nil {
		return plan, errors.New("parser-core phase zero: EOF recovery source has no immutable reduction-plan provider")
	}
	demand, err := diagnosticEOFRecoveryValidationScratchDemand(live, payloads)
	if err != nil {
		return plan, err
	}
	plan.validationDemand = demand

	for _, item := range []struct {
		count int
		bytes uint64
	}{
		{len(live.nodes), coreNodeRecordBytes},
		{len(live.nodeLineages), coreNodeLineageRecordBytes},
		{len(live.nodeCheckpoints), coreCheckpointIDBytes},
		{len(live.links), coreLinkRecordBytes},
		{len(live.subtrees), coreSubtreeRecordBytes},
		{len(live.eofRecoveryRoots), coreSubtreeIDBytes},
		{len(live.externalProvenance), coreExternalProvenanceBytes},
		{len(live.lexerSkippedPrefixes), coreLexerSkippedPrefixBytes},
		{len(live.children), coreChildRecordBytes},
		{len(live.fields), coreFieldRecordBytes},
		{len(live.aliases), coreAliasRecordBytes},
		{len(live.checkpoints.records), coreCheckpointRecordBytes},
		{len(live.checkpoints.bytes), 1},
		{len(live.boundaries.slots), coreBoundarySlotBytes},
		{len(live.alternativeSpillArena), coreUint32Bytes},
		{len(live.dropCohortRefSpill), coreDropCohortRefBytes},
		{len(live.dropCohortActions), coreDropCohortActionBytes},
		{len(live.dropCohortRecords), coreDropCohortRecordBytes},
		{len(live.dropCohortMembers), coreDropCohortMemberBytes},
		{len(live.dropCohortDerivations), coreDropCohortDerivationRecordBytes},
		{len(live.dropCohortDerivationBytes), 1},
	} {
		bytes, ok := diagnosticEOFRecoveryMul(uint64(item.count), item.bytes)
		if !ok || !diagnosticEOFRecoveryAdd(&plan.copiedArenaBytes, bytes) {
			return plan, errors.New("parser-core phase zero: EOF recovery copied-arena byte count overflow")
		}
	}
	if !diagnosticEOFRecoveryAdd(&plan.appendReserveBytes, coreSubtreeRecordBytes) {
		return plan, errors.New("parser-core phase zero: EOF recovery subtree reserve overflow")
	}
	if !diagnosticEOFRecoveryAdd(&plan.appendReserveBytes, coreSubtreeIDBytes) {
		return plan, errors.New("parser-core phase zero: EOF recovery root marker reserve overflow")
	}
	childReserve, ok := diagnosticEOFRecoveryMul(uint64(len(payloads)), coreChildRecordBytes)
	if !ok || !diagnosticEOFRecoveryAdd(&plan.appendReserveBytes, childReserve) {
		return plan, errors.New("parser-core phase zero: EOF recovery child reserve overflow")
	}
	for _, item := range []struct {
		count int
		bytes uint64
	}{
		{demand.structuralPositions, coreUint16Bytes},
		{demand.remappedFields, coreFieldRecordBytes},
		{demand.remappedAliases, coreAliasRecordBytes},
	} {
		bytes, ok := diagnosticEOFRecoveryMul(uint64(item.count), item.bytes)
		if !ok || !diagnosticEOFRecoveryAdd(&plan.temporaryBytes, bytes) {
			return plan, errors.New("parser-core phase zero: EOF recovery validation scratch byte count overflow")
		}
	}

	plan.peakBytes = plan.coreHeaderBytes
	for _, bytes := range []uint64{
		plan.copiedArenaBytes,
		plan.appendReserveBytes,
		plan.mapBytes,
		plan.temporaryBytes,
		plan.preservationBytes,
		plan.providerWrapperBytes,
	} {
		if !diagnosticEOFRecoveryAdd(&plan.peakBytes, bytes) {
			return plan, errors.New("parser-core phase zero: EOF recovery peak byte count overflow")
		}
	}
	if plan.peakBytes > DiagnosticEOFRecoveryMaxCloneBytes {
		return plan, fmt.Errorf(
			"parser-core phase zero: EOF recovery clone peak %d exceeds %d",
			plan.peakBytes, DiagnosticEOFRecoveryMaxCloneBytes,
		)
	}
	return plan, nil
}

func diagnosticEOFRecoveryValidationScratchDemand(
	live *Core,
	payloads []SubtreeID,
) (diagnosticEOFRecoveryValidationDemand, error) {
	demand := diagnosticEOFRecoveryValidationDemand{}
	for index := range live.subtrees {
		record := live.subtrees[index]
		if record.terminal {
			continue
		}
		end := uint64(record.firstChild) + uint64(record.childCount)
		if end > uint64(len(live.children)) {
			return demand, errors.New("parser-core phase zero: EOF recovery source child range is invalid")
		}
		children := live.children[record.firstChild:end]
		if err := diagnosticEOFRecoveryObserveValidationDemand(
			live,
			record.productionID,
			children,
			int(record.fieldCount),
			int(record.aliasCount),
			false,
			&demand,
		); err != nil {
			return demand, err
		}
	}
	if err := diagnosticEOFRecoveryObserveValidationDemand(
		live,
		0,
		payloads,
		0,
		0,
		true,
		&demand,
	); err != nil {
		return demand, err
	}
	return demand, nil
}

func diagnosticEOFRecoveryObserveValidationDemand(
	live *Core,
	productionID uint16,
	children []SubtreeID,
	storedFieldCount int,
	storedAliasCount int,
	syntheticRoot bool,
	demand *diagnosticEOFRecoveryValidationDemand,
) error {
	structuralCount := 0
	for _, child := range children {
		record, err := live.subtree(child)
		if err != nil {
			return err
		}
		if !record.extra {
			structuralCount++
		}
	}
	plan, err := live.reductionPlanForPair(productionID, structuralCount)
	if err != nil {
		return err
	}
	wantAliases := 0
	if len(plan.aliases) != 0 {
		wantAliases = len(children)
	}
	if syntheticRoot {
		if len(plan.fields) != 0 || wantAliases != 0 {
			return errors.New("parser-core phase zero: EOF recovery ERROR root requires empty production metadata")
		}
	} else if storedFieldCount != len(plan.fields) || storedAliasCount != wantAliases {
		return errors.New("parser-core phase zero: EOF recovery source metadata counts do not match authenticated plans")
	}
	demand.structuralPositions = max(demand.structuralPositions, structuralCount)
	demand.remappedFields = max(demand.remappedFields, len(plan.fields))
	demand.remappedAliases = max(demand.remappedAliases, wantAliases)
	return nil
}

func cloneDiagnosticEOFRecoveryCore(
	live *Core,
	payloadCount int,
	validationDemand diagnosticEOFRecoveryValidationDemand,
) *Core {
	shadow := new(Core)
	*shadow = *live
	shadow.tables = nil
	shadow.plans = nil
	shadow.selectedProvider = nil
	shadow.selectedPolicy = nil
	shadow.nodes = cloneDiagnosticSlice(live.nodes, 0)
	shadow.nodeLineages = cloneDiagnosticSlice(live.nodeLineages, 0)
	shadow.nodeCheckpoints = cloneDiagnosticSlice(live.nodeCheckpoints, 0)
	shadow.links = cloneDiagnosticSlice(live.links, 0)
	shadow.subtrees = cloneDiagnosticSlice(live.subtrees, 1)
	shadow.eofRecoveryRoots = cloneDiagnosticSlice(live.eofRecoveryRoots, 1)
	shadow.externalProvenance = cloneDiagnosticSlice(live.externalProvenance, 0)
	shadow.lexerSkippedPrefixes = cloneDiagnosticSlice(live.lexerSkippedPrefixes, 0)
	shadow.children = cloneDiagnosticSlice(live.children, payloadCount)
	shadow.fields = cloneDiagnosticSlice(live.fields, 0)
	shadow.aliases = cloneDiagnosticSlice(live.aliases, 0)
	shadow.checkpoints.records = cloneDiagnosticSlice(live.checkpoints.records, 0)
	shadow.checkpoints.bytes = cloneDiagnosticSlice(live.checkpoints.bytes, 0)
	shadow.checkpoints.buckets = nil
	shadow.boundaries.slots = cloneDiagnosticSlice(live.boundaries.slots, 0)
	shadow.alternativeSpillArena = cloneDiagnosticSlice(live.alternativeSpillArena, 0)
	shadow.dropCohortRefSpill = cloneDiagnosticSlice(live.dropCohortRefSpill, 0)
	shadow.dropCohortActions = cloneDiagnosticSlice(live.dropCohortActions, 0)
	shadow.dropCohortRecords = cloneDiagnosticSlice(live.dropCohortRecords, 0)
	shadow.dropCohortMembers = cloneDiagnosticSlice(live.dropCohortMembers, 0)
	shadow.dropCohortDerivations = cloneDiagnosticSlice(live.dropCohortDerivations, 0)
	shadow.dropCohortDerivationBytes = cloneDiagnosticSlice(live.dropCohortDerivationBytes, 0)

	// The EOF recovery append path does not read or mutate these drop-cohort
	// families. Clear them after the shallow Core copy so no omitted sidecar,
	// nested reservation header, or map can alias live mutable state.
	shadow.dropCohortLinkRefIndexes = nil
	shadow.dropCohortLinkRefJournal = nil
	shadow.dropCohortDerivationIntern = nil
	shadow.dropCohortCertificateRefs = nil
	shadow.dropCohortMapStore = nil
	shadow.dropCohortJournalStore = nil
	shadow.dropCohortFrontiers = nil
	shadow.dropCohortFrontierParticipants = nil
	shadow.dropCohortFrontierMembers = nil
	shadow.dropCohortFrontierJournal = nil
	shadow.dropCohortReservations = nil

	// One append does not use live journals, schedulers, or retained builders.
	shadow.boundaryJournal = nil
	shadow.nodeLineageJournal = nil
	shadow.dropCohortJournal = nil
	shadow.dropCohortDerivationScratch = nil
	shadow.dropCohortPathScratch = nil
	shadow.condenseCandidates = nil
	shadow.condenseNewNode = 0
	shadow.condenseScopeActive = false
	shadow.reductionSourceOwner = 0
	shadow.transactions = nil
	shadow.popScratch = popEnumerationScratch{}
	shadow.reductionScratch = reductionOutputScratch{
		structuralPositions: make([]uint16, 0, validationDemand.structuralPositions),
		remappedFields:      make([]FieldMapEntry, 0, validationDemand.remappedFields),
		remappedAliases:     make([]Symbol, 0, validationDemand.remappedAliases),
	}
	shadow.historicalNodeScratch = nil
	shadow.cohortHeadScratch = nil
	shadow.factorLinkScratch = nil
	shadow.selectedBuild = selectedStoreBuildScratch{}
	shadow.selectedPoolMu = sync.Mutex{}
	shadow.selectedPool = selectedStoreBacking{}
	shadow.schedulerFrame = schedulerTransactionFrame{}
	return shadow
}

// DiagnosticEOFRecoveryValidationScratch returns the current Core-owned
// validation capacities. The diagnostic caller compares this value before and
// after materialization to reject unplanned growth.
func (c *Core) DiagnosticEOFRecoveryValidationScratch() DiagnosticEOFRecoveryValidationScratch {
	if c == nil {
		return DiagnosticEOFRecoveryValidationScratch{}
	}
	return DiagnosticEOFRecoveryValidationScratch{
		StructuralPositions: uint32(cap(c.reductionScratch.structuralPositions)),
		RemappedFields:      uint32(cap(c.reductionScratch.remappedFields)),
		RemappedAliases:     uint32(cap(c.reductionScratch.remappedAliases)),
	}
}

func cloneDiagnosticSlice[T any](source []T, extraCapacity int) []T {
	if len(source) == 0 && extraCapacity == 0 {
		return nil
	}
	out := make([]T, len(source), len(source)+extraCapacity)
	copy(out, source)
	return out
}

func diagnosticEOFRecoveryCopiedArenasEqual(live, shadow *Core) bool {
	if live == nil || shadow == nil || len(shadow.subtrees) < len(live.subtrees) ||
		len(shadow.eofRecoveryRoots) < len(live.eofRecoveryRoots) ||
		len(shadow.children) < len(live.children) {
		return false
	}
	return equalDiagnosticSlice(live.nodes, shadow.nodes) &&
		equalDiagnosticSlice(live.nodeLineages, shadow.nodeLineages) &&
		equalDiagnosticSlice(live.nodeCheckpoints, shadow.nodeCheckpoints) &&
		equalDiagnosticSlice(live.links, shadow.links) &&
		equalDiagnosticSlice(live.subtrees, shadow.subtrees[:len(live.subtrees)]) &&
		equalDiagnosticSlice(live.eofRecoveryRoots, shadow.eofRecoveryRoots[:len(live.eofRecoveryRoots)]) &&
		equalDiagnosticSlice(live.externalProvenance, shadow.externalProvenance) &&
		equalDiagnosticSlice(live.lexerSkippedPrefixes, shadow.lexerSkippedPrefixes) &&
		equalDiagnosticSlice(live.children, shadow.children[:len(live.children)]) &&
		equalDiagnosticSlice(live.fields, shadow.fields) &&
		equalDiagnosticSlice(live.aliases, shadow.aliases) &&
		equalDiagnosticSlice(live.checkpoints.records, shadow.checkpoints.records) &&
		equalDiagnosticSlice(live.checkpoints.bytes, shadow.checkpoints.bytes) &&
		equalDiagnosticSlice(live.boundaries.slots, shadow.boundaries.slots) &&
		equalDiagnosticSlice(live.alternativeSpillArena, shadow.alternativeSpillArena) &&
		equalDiagnosticSlice(live.dropCohortRefSpill, shadow.dropCohortRefSpill) &&
		equalDiagnosticSlice(live.dropCohortActions, shadow.dropCohortActions) &&
		equalDiagnosticSlice(live.dropCohortRecords, shadow.dropCohortRecords) &&
		equalDiagnosticSlice(live.dropCohortMembers, shadow.dropCohortMembers) &&
		equalDiagnosticSlice(live.dropCohortDerivations, shadow.dropCohortDerivations) &&
		equalDiagnosticSlice(live.dropCohortDerivationBytes, shadow.dropCohortDerivationBytes)
}

func diagnosticEOFRecoveryCopiedHeadersEqual(live, shadow *Core) bool {
	if live == nil || shadow == nil {
		return false
	}
	return live.limits == shadow.limits &&
		live.diagnostics == shadow.diagnostics &&
		live.frontier == shadow.frontier &&
		live.checkpoint == shadow.checkpoint &&
		live.nextTransaction == shadow.nextTransaction &&
		live.classificationPhase == shadow.classificationPhase &&
		live.metadataConstructionAuthenticated == shadow.metadataConstructionAuthenticated &&
		live.reduceConflictContext == shadow.reduceConflictContext &&
		live.reduceNoLookaheadContext == shadow.reduceNoLookaheadContext &&
		live.externalPayloadsQuiescent == shadow.externalPayloadsQuiescent &&
		live.terminalScannerCheckpointProvenance == shadow.terminalScannerCheckpointProvenance &&
		live.externalTokenScannerStart == shadow.externalTokenScannerStart &&
		live.externalTokenScannerEnd == shadow.externalTokenScannerEnd &&
		live.externalTokenScannerExact == shadow.externalTokenScannerExact
}

func diagnosticEOFRecoveryRootChildrenEqual(shadow *Core, root SubtreeID, payloads []SubtreeID) bool {
	record, err := shadow.subtree(root)
	if err != nil || uint64(record.firstChild)+uint64(record.childCount) > uint64(len(shadow.children)) {
		return false
	}
	children := shadow.children[record.firstChild : record.firstChild+record.childCount]
	return equalDiagnosticSlice(children, payloads)
}

func diagnosticEOFRecoveryStorageDisjoint(live, shadow *Core) bool {
	if live == nil || shadow == nil || live == shadow {
		return false
	}
	return disjointDiagnosticSlice(live.nodes, shadow.nodes) &&
		disjointDiagnosticSlice(live.nodeLineages, shadow.nodeLineages) &&
		disjointDiagnosticSlice(live.nodeCheckpoints, shadow.nodeCheckpoints) &&
		disjointDiagnosticSlice(live.links, shadow.links) &&
		disjointDiagnosticSlice(live.subtrees, shadow.subtrees) &&
		disjointDiagnosticSlice(live.eofRecoveryRoots, shadow.eofRecoveryRoots) &&
		disjointDiagnosticSlice(live.externalProvenance, shadow.externalProvenance) &&
		disjointDiagnosticSlice(live.lexerSkippedPrefixes, shadow.lexerSkippedPrefixes) &&
		disjointDiagnosticSlice(live.children, shadow.children) &&
		disjointDiagnosticSlice(live.fields, shadow.fields) &&
		disjointDiagnosticSlice(live.aliases, shadow.aliases) &&
		disjointDiagnosticSlice(live.checkpoints.records, shadow.checkpoints.records) &&
		disjointDiagnosticSlice(live.checkpoints.bytes, shadow.checkpoints.bytes) &&
		disjointDiagnosticSlice(live.boundaries.slots, shadow.boundaries.slots) &&
		disjointDiagnosticSlice(live.alternativeSpillArena, shadow.alternativeSpillArena) &&
		disjointDiagnosticSlice(live.dropCohortRefSpill, shadow.dropCohortRefSpill) &&
		disjointDiagnosticSlice(live.dropCohortActions, shadow.dropCohortActions) &&
		disjointDiagnosticSlice(live.dropCohortRecords, shadow.dropCohortRecords) &&
		disjointDiagnosticSlice(live.dropCohortMembers, shadow.dropCohortMembers) &&
		disjointDiagnosticSlice(live.dropCohortDerivations, shadow.dropCohortDerivations) &&
		disjointDiagnosticSlice(live.dropCohortDerivationBytes, shadow.dropCohortDerivationBytes)
}

func disjointDiagnosticSlice[T any](left, right []T) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	return &left[0] != &right[0]
}

func equalDiagnosticSlice[T comparable](left, right []T) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func diagnosticEOFRecoveryMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

func diagnosticEOFRecoveryAdd(total *uint64, value uint64) bool {
	if total == nil || value > math.MaxUint64-*total {
		return false
	}
	*total += value
	return true
}
