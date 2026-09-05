//go:build gts_eof_recovery_shadow

package parsercorephase0

import (
	"errors"
	"strings"
	"testing"
	"unsafe"
)

func TestDiagnosticEOFRecoveryCloneDetachesVisibleCountMemo(t *testing.T) {
	live := newRecoveryVisibleCore(t)
	leaf := appendRecoveryVisibleFixture(t, live, ordinarySymbol, false, false, nil, nil)
	symbols := visibleSymbols()
	if got, err := live.CachedVisibleSubtreeCount(symbols, leaf); err != nil || got != 1 {
		t.Fatalf("live count=%d/%v, want one", got, err)
	}
	shadow := cloneDiagnosticEOFRecoveryCore(live, 0, diagnosticEOFRecoveryValidationDemand{})
	if shadow.recoveryVisibleCounts != nil || shadow.recoveryVisibleSymbols != nil {
		t.Fatal("shadow retained the live visible-count cache")
	}
	symbols[ordinarySymbol].Visible = false
	if got, err := shadow.CachedVisibleSubtreeCount(symbols, leaf); err != nil || got != 0 {
		t.Fatalf("shadow count=%d/%v, want zero", got, err)
	}
	if live.recoveryVisibleCounts[leaf-1].count != 1 || !live.recoveryVisibleSymbols[ordinarySymbol] {
		t.Fatal("shadow counting changed the live cache")
	}
	if !diagnosticEOFRecoveryStorageDisjoint(live, shadow) {
		t.Fatal("shadow retained shared mutable storage")
	}
}

func TestDiagnosticEOFRecoveryClonePlanAccountsEveryRequestedByte(t *testing.T) {
	live := &Core{
		metadataConstructionAuthenticated: true,
		plans:                             &diagnosticEOFRecoveryPlanOnlyProvider{identity: 1},
		nodes:                             make([]nodeRecord, 2),
		nodeLineages:                      make([]nodeLineageRecord, 2),
		nodeCheckpoints:                   make([]CheckpointID, 2),
		links:                             make([]linkRecord, 3),
		subtrees:                          make([]subtreeRecord, 4),
		eofRecoveryRoots:                  []SubtreeID{2},
		externalProvenance:                make([]externalPayloadProvenance, 1),
		lexerSkippedPrefixes:              make([]lexerSkippedPrefixProvenance, 2),
		children:                          make([]SubtreeID, 5),
		fields:                            make([]FieldMapEntry, 6),
		aliases:                           make([]Symbol, 7),
		checkpoints: checkpointInterner{
			records: make([]checkpointRecord, 2),
			bytes:   make([]byte, 9),
		},
		boundaries:            boundaryIndex{slots: make([]boundarySlot, 8)},
		alternativeSpillArena: make([]uint32, 10),
	}
	for index := range live.subtrees {
		live.subtrees[index].terminal = true
	}
	const (
		providerWrapperBytes = 24
	)
	payloads := []SubtreeID{1, 2, 3, 4}
	plan, err := planDiagnosticEOFRecoveryClone(live, payloads, providerWrapperBytes)
	if err != nil {
		t.Fatalf("plan clone: %v", err)
	}
	wantCopied := uint64(2)*coreNodeRecordBytes +
		uint64(2)*coreNodeLineageRecordBytes +
		uint64(2)*coreCheckpointIDBytes +
		uint64(3)*coreLinkRecordBytes +
		uint64(4)*coreSubtreeRecordBytes +
		coreSubtreeIDBytes +
		coreExternalProvenanceBytes +
		uint64(2)*coreLexerSkippedPrefixBytes +
		uint64(5)*coreChildRecordBytes +
		uint64(6)*coreFieldRecordBytes +
		uint64(7)*coreAliasRecordBytes +
		uint64(2)*coreCheckpointRecordBytes +
		9 +
		uint64(8)*coreBoundarySlotBytes +
		uint64(10)*coreUint32Bytes
	wantAppend := coreSubtreeRecordBytes + uint64(len(payloads))*coreChildRecordBytes + coreSubtreeIDBytes
	wantTemporary := uint64(len(payloads)) * coreUint16Bytes
	wantPeak := uint64(unsafe.Sizeof(Core{})) + wantCopied + wantAppend + wantTemporary + providerWrapperBytes
	if plan.coreHeaderBytes != uint64(unsafe.Sizeof(Core{})) ||
		plan.copiedArenaBytes != wantCopied || plan.appendReserveBytes != wantAppend ||
		plan.mapBytes != 0 || plan.temporaryBytes != wantTemporary || plan.preservationBytes != 0 ||
		plan.providerWrapperBytes != providerWrapperBytes ||
		plan.validationDemand != (diagnosticEOFRecoveryValidationDemand{structuralPositions: len(payloads)}) ||
		plan.peakBytes != wantPeak {
		t.Fatalf("clone plan=%+v want copied=%d append=%d peak=%d", plan, wantCopied, wantAppend, wantPeak)
	}
}

func TestDiagnosticEOFRecoveryClonePlanRejectsRetainedState(t *testing.T) {
	live := &Core{metadataConstructionAuthenticated: true, selectedPolicy: &SelectedStorePolicy{}}
	if _, err := planDiagnosticEOFRecoveryClone(live, []SubtreeID{1}, 8); err == nil || !strings.Contains(err.Error(), "selected policy") {
		t.Fatalf("retained selected policy error=%v", err)
	}
	live.selectedPolicy = nil
	live.checkpoints.buckets = map[[32]byte]CheckpointID{{1}: 1}
	if _, err := planDiagnosticEOFRecoveryClone(live, []SubtreeID{1}, 8); err == nil || !strings.Contains(err.Error(), "checkpoint map") {
		t.Fatalf("nonempty checkpoint map error=%v", err)
	}
	live.checkpoints.buckets = nil
	if _, err := planDiagnosticEOFRecoveryClone(live, []SubtreeID{1}, 0); err == nil || !strings.Contains(err.Error(), "provider wrapper storage") {
		t.Fatalf("unaccounted provider wrapper error=%v", err)
	}
}

func TestDiagnosticEOFRecoveryCopiedArenasEqualRejectsPartialRootSidecar(t *testing.T) {
	live := &Core{eofRecoveryRoots: []SubtreeID{1, 2}}
	shadow := &Core{eofRecoveryRoots: []SubtreeID{1}}
	if diagnosticEOFRecoveryCopiedArenasEqual(live, shadow) {
		t.Fatal("partial EOF recovery root sidecar was accepted")
	}
}

func TestDiagnosticEOFRecoveryCloneClearsUnaccountedMutableDropCohortState(t *testing.T) {
	live := &Core{
		checkpoints: checkpointInterner{
			buckets: map[[32]byte]CheckpointID{{1}: 1},
		},
		reductionScratch: reductionOutputScratch{
			boundaryByKey: map[boundaryKey]int{{state: 1}: 1},
		},
		dropCohortLinkRefIndexes:       []uint32{1},
		dropCohortLinkRefJournal:       []dropCohortLinkRefMutation{{}},
		dropCohortDerivationIntern:     []dropCohortDerivationInternEntry{{}},
		dropCohortCertificateRefs:      []DropCohortRef{{}},
		dropCohortMapStore:             []dropCohortMapEntry{{}},
		dropCohortJournalStore:         []dropCohortJournalStoreEntry{{}},
		dropCohortFrontiers:            []dropCohortFrontierRecord{{}},
		dropCohortFrontierParticipants: []dropCohortFrontierParticipant{{}},
		dropCohortFrontierMembers:      []dropCohortFrontierMember{{}},
		dropCohortFrontierJournal:      []dropCohortFrontierMutation{{}},
		dropCohortDerivationScratch:    []byte{1},
		dropCohortPathScratch:          []dropCohortPathStep{{}},
		dropCohortJournal:              []dropCohortMutation{{}},
		dropCohortReservations:         []dropCohortReservation{{actionsHeader: []dropCohortActionIdentity{{}}}},
	}
	shadow := cloneDiagnosticEOFRecoveryCore(live, 1, diagnosticEOFRecoveryValidationDemand{})
	if shadow.checkpoints.buckets != nil || shadow.reductionScratch.boundaryByKey != nil {
		t.Fatal("clone retained mutable map state")
	}
	for name, length := range map[string]int{
		"link ref indexes":      len(shadow.dropCohortLinkRefIndexes),
		"link ref journal":      len(shadow.dropCohortLinkRefJournal),
		"derivation interner":   len(shadow.dropCohortDerivationIntern),
		"certificate refs":      len(shadow.dropCohortCertificateRefs),
		"map store":             len(shadow.dropCohortMapStore),
		"journal store":         len(shadow.dropCohortJournalStore),
		"frontiers":             len(shadow.dropCohortFrontiers),
		"frontier participants": len(shadow.dropCohortFrontierParticipants),
		"frontier members":      len(shadow.dropCohortFrontierMembers),
		"frontier journal":      len(shadow.dropCohortFrontierJournal),
		"derivation scratch":    len(shadow.dropCohortDerivationScratch),
		"path scratch":          len(shadow.dropCohortPathScratch),
		"journal":               len(shadow.dropCohortJournal),
		"reservations":          len(shadow.dropCohortReservations),
	} {
		if length != 0 {
			t.Fatalf("clone retained %s length %d", name, length)
		}
	}
	if !diagnosticEOFRecoveryStorageDisjoint(live, shadow) {
		t.Fatal("clone storage was not disjoint after clearing unused state")
	}
}

func TestDiagnosticEOFRecoveryForkClearsMetadataAuthentication(t *testing.T) {
	live, err := New(&fakeTable{}, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	live.plans = &diagnosticEOFRecoveryPlanOnlyProvider{identity: 1}
	head, err := live.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err = live.appendDiagnosticPayload(head, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := live.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(paths[0].Payloads) != 1 || !live.metadataConstructionAuthenticated {
		t.Fatalf("live setup paths=%+v authenticated=%t", paths, live.metadataConstructionAuthenticated)
	}

	shadow, _, receipt, err := ForkDiagnosticEOFRecovery(live, head, paths[0].Payloads, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.CopiedHeadersEqual || !receipt.MetadataConstructionUnauthenticated {
		t.Fatalf("metadata receipt=%+v", receipt)
	}
	if shadow.metadataConstructionAuthenticated {
		t.Fatal("diagnostic recovery publication retained metadata authentication")
	}
	if !live.metadataConstructionAuthenticated {
		t.Fatal("diagnostic recovery publication changed the live metadata authentication")
	}
}

type diagnosticEOFRecoveryPlanOnlyProvider struct {
	identity byte
}

func (*diagnosticEOFRecoveryPlanOnlyProvider) ReductionPlan(productionID uint16, childCount int) (ReductionPlan, error) {
	return NewReductionPlan(productionID, childCount, nil, nil)
}

type diagnosticEOFRecoveryScratchProvider struct {
	identity byte
	parent   ReductionPlan
	root     ReductionPlan
}

func (p *diagnosticEOFRecoveryScratchProvider) ReductionPlan(productionID uint16, childCount int) (ReductionPlan, error) {
	if p != nil && productionID == 7 && childCount == 2 {
		return p.parent, nil
	}
	if p != nil && productionID == 0 && childCount == 1 {
		return p.root, nil
	}
	return ReductionPlan{}, errors.New("unexpected test reduction plan pair")
}

func TestDiagnosticEOFRecoveryForkReservesValidationScratchWithoutGrowth(t *testing.T) {
	parentPlan, err := NewReductionPlan(
		7,
		2,
		[]FieldMapEntry{{FieldID: 1, ChildIndex: 1}},
		[]Symbol{11, 12},
	)
	if err != nil {
		t.Fatal(err)
	}
	rootPlan, err := NewReductionPlan(0, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceProvider := &diagnosticEOFRecoveryScratchProvider{identity: 1, parent: parentPlan, root: rootPlan}
	live := &Core{
		tables:                            &fakeTable{},
		plans:                             sourceProvider,
		limits:                            (Limits{}).withDefaults(),
		metadataConstructionAuthenticated: true,
		nodes:                             []nodeRecord{{}},
		subtrees: []subtreeRecord{
			{terminal: true, startByte: 0, endByte: 1},
			{terminal: true, extra: true, startByte: 1, endByte: 2},
			{terminal: true, startByte: 2, endByte: 3},
			{
				productionID: 7, startByte: 0, endByte: 3,
				firstChild: 0, childCount: 3,
				firstField: 0, fieldCount: 1,
				firstAlias: 0, aliasCount: 3,
			},
		},
		children: []SubtreeID{1, 2, 3},
		fields:   []FieldMapEntry{{FieldID: 1, ChildIndex: 2}},
		aliases:  []Symbol{11, 0, 12},
	}

	shadow, root, receipt, err := ForkDiagnosticEOFRecovery(live, Head{Node: 1}, []SubtreeID{4}, 8)
	if err != nil {
		t.Fatal(err)
	}
	wantScratch := DiagnosticEOFRecoveryValidationScratch{
		StructuralPositions: 2,
		RemappedFields:      1,
		RemappedAliases:     3,
	}
	if !receipt.ValidationScratchReserved || shadow.DiagnosticEOFRecoveryValidationScratch() != wantScratch {
		t.Fatalf("reserved validation scratch receipt=%+v caps=%+v want=%+v", receipt, shadow.DiagnosticEOFRecoveryValidationScratch(), wantScratch)
	}
	wantTemporary := uint64(2)*coreUint16Bytes + coreFieldRecordBytes + uint64(3)*coreAliasRecordBytes
	if receipt.TemporaryBytes != wantTemporary {
		t.Fatalf("validation scratch bytes=%d want=%d", receipt.TemporaryBytes, wantTemporary)
	}
	shadowProvider := &diagnosticEOFRecoveryScratchProvider{identity: 2, parent: parentPlan, root: rootPlan}
	if _, err := shadow.BindDiagnosticEOFRecoveryReductionPlans(live, shadowProvider); err != nil {
		t.Fatal(err)
	}
	before := shadow.DiagnosticEOFRecoveryValidationScratch()
	if err := shadow.validateGenericMaterializationMetadata(4, shadow.subtrees[3]); err != nil {
		t.Fatalf("validate copied parent: %v", err)
	}
	if err := shadow.validateGenericMaterializationMetadata(root, shadow.subtrees[root-1]); err != nil {
		t.Fatalf("validate synthetic root: %v", err)
	}
	after := shadow.DiagnosticEOFRecoveryValidationScratch()
	if after != before {
		t.Fatalf("validation grew scratch: before=%+v after=%+v", before, after)
	}
}

type diagnosticEOFRecoveryWideProvider struct {
	diagnosticEOFRecoveryPlanOnlyProvider
}

func (*diagnosticEOFRecoveryWideProvider) Actions(StateID, Symbol) (ActionRow, error) {
	return ActionRow{}, nil
}

func (*diagnosticEOFRecoveryWideProvider) Goto(StateID, Symbol) (StateID, error) {
	return 0, nil
}

func (*diagnosticEOFRecoveryWideProvider) ProductionFields(uint16, int) ([]FieldMapEntry, error) {
	return nil, nil
}

func (*diagnosticEOFRecoveryWideProvider) ProductionAliases(uint16, int) ([]Symbol, error) {
	return nil, nil
}

type diagnosticEOFRecoverySelectedProvider struct {
	diagnosticEOFRecoveryPlanOnlyProvider
}

func (*diagnosticEOFRecoverySelectedProvider) SelectedStorePolicy() (SelectedStorePolicy, error) {
	return SelectedStorePolicy{}, nil
}

func TestDiagnosticEOFRecoveryProviderBindRejectsAliasesAndWideCapabilities(t *testing.T) {
	sourcePlan := &diagnosticEOFRecoveryPlanOnlyProvider{identity: 1}
	source := &Core{tables: &fakeTable{}, plans: sourcePlan}
	newShadow := func() *Core { return &Core{} }

	if _, err := newShadow().BindDiagnosticEOFRecoveryReductionPlans(source, nil); err == nil || !strings.Contains(err.Error(), "is nil") {
		t.Fatalf("nil provider error=%v", err)
	}
	if _, err := newShadow().BindDiagnosticEOFRecoveryReductionPlans(source, sourcePlan); err == nil || !strings.Contains(err.Error(), "aliases") {
		t.Fatalf("source alias error=%v", err)
	}
	if _, err := newShadow().BindDiagnosticEOFRecoveryReductionPlans(source, &diagnosticEOFRecoveryWideProvider{}); err == nil || !strings.Contains(err.Error(), "table view") {
		t.Fatalf("wide table provider error=%v", err)
	}
	if _, err := newShadow().BindDiagnosticEOFRecoveryReductionPlans(source, &diagnosticEOFRecoverySelectedProvider{}); err == nil || !strings.Contains(err.Error(), "selected-store") {
		t.Fatalf("selected-store provider error=%v", err)
	}
	prebound := newShadow()
	prebound.plans = &diagnosticEOFRecoveryPlanOnlyProvider{identity: 2}
	if _, err := prebound.BindDiagnosticEOFRecoveryReductionPlans(source, &diagnosticEOFRecoveryPlanOnlyProvider{identity: 3}); err == nil || !strings.Contains(err.Error(), "prebound") {
		t.Fatalf("prebound provider error=%v", err)
	}

	shadow := newShadow()
	receipt, err := shadow.BindDiagnosticEOFRecoveryReductionPlans(source, &diagnosticEOFRecoveryPlanOnlyProvider{identity: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.SourceProvidersDetached || !receipt.ReductionPlansAttached ||
		!receipt.ProviderDiffersFromSource || !receipt.TableViewDetached || !receipt.SelectedStoreDetached {
		t.Fatalf("provider bind receipt=%+v", receipt)
	}
	if shadow.tables != nil || shadow.plans == nil || shadow.selectedProvider != nil {
		t.Fatalf("provider slots tables=%v plans=%v selected=%v", shadow.tables, shadow.plans, shadow.selectedProvider)
	}
}
