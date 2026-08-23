//go:build gts_parsercorephase0

package gotreesitter

import (
	"fmt"
	"slices"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func g18RootRefs(t *testing.T, compact *core.Core) core.DropCohortRefSet {
	t.Helper()
	var refs core.DropCohortRefSet
	for _, ref := range []core.DropCohortRef{
		{Owner: 7, Epoch: 2, Sequence: 3, Branch: 1},
		{Owner: 7, Epoch: 2, Sequence: 3, Branch: 0},
		{Owner: 8, Epoch: 1, Sequence: 1, Branch: 0},
	} {
		if !compact.AddDropCohortRef(&refs, ref) {
			t.Fatalf("add drop-cohort reference %+v failed", ref)
		}
	}
	return refs
}

func g18RootMembers(t *testing.T, compact *core.Core, refs core.DropCohortRefSet) []core.DropCohortRef {
	t.Helper()
	members := make([]core.DropCohortRef, refs.Len())
	for index := range members {
		var ok bool
		members[index], ok = compact.DropCohortRefAt(refs, index)
		if !ok {
			t.Fatalf("drop-cohort reference %d is invalid: %+v", index, refs)
		}
	}
	return members
}

func TestG18RefSetHeaderAndCanonicalPropagation(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	refs := g18RootRefs(t, compact)
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: head, dropCohortRefs: refs, creationSeq: 1, frontierSequence: 17},
		{head: head, dropCohortRefs: refs, creationSeq: 2, frontierSequence: 17},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || !slices.Equal(g18RootMembers(t, compact, out[0].dropCohortRefs), g18RootMembers(t, compact, refs)) {
		t.Fatalf("linear canonical refs=%+v output=%+v", refs, out)
	}
	if out[0].frontierSequence != 17 {
		t.Fatalf("linear canonical frontier=%d, want 17", out[0].frontierSequence)
	}

	// Force the mapped path and fold a distinct reference into its winner.
	heads := make([]core.Head, 9)
	for index := range heads {
		heads[index], err = compact.Seed(core.StateID(index+10), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	other := core.DropCohortRefSet{}
	if !compact.AddDropCohortRef(&other, core.DropCohortRef{Owner: 9, Epoch: 1, Sequence: 1, Branch: 0}) {
		t.Fatal("add mapped reference failed")
	}
	headers := make([]diagnosticParserCoreHeader, 0, 10)
	for index, seeded := range heads {
		set := refs
		if index == 0 {
			set = other
		}
		headers = append(headers, diagnosticParserCoreHeader{head: seeded, dropCohortRefs: set, creationSeq: uint64(index + 1), frontierSequence: 23})
	}
	headers = append(headers, diagnosticParserCoreHeader{head: heads[0], dropCohortRefs: refs, creationSeq: 99, frontierSequence: 23})
	out, err = scratch.canonicalize(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 9 || scratch.groups == nil {
		t.Fatalf("mapped canonical output len=%d groups=%v", len(out), scratch.groups != nil)
	}
	var winner diagnosticParserCoreHeader
	for _, header := range out {
		if header.head == heads[0] {
			winner = header
			break
		}
	}
	if winner.head != heads[0] {
		t.Fatalf("mapped canonical output omitted the duplicate winner: %+v", out)
	}
	if winner.frontierSequence != 23 {
		t.Fatalf("mapped canonical frontier=%d, want 23", winner.frontierSequence)
	}
	members := g18RootMembers(t, compact, winner.dropCohortRefs)
	want := append(g18RootMembers(t, compact, refs), g18RootMembers(t, compact, other)...)
	slices.SortFunc(want, func(left, right core.DropCohortRef) int {
		if left.Owner != right.Owner {
			if left.Owner < right.Owner {
				return -1
			}
			return 1
		}
		if left.Epoch != right.Epoch {
			if left.Epoch < right.Epoch {
				return -1
			}
			return 1
		}
		if left.Sequence != right.Sequence {
			if left.Sequence < right.Sequence {
				return -1
			}
			return 1
		}
		if left.Branch < right.Branch {
			return -1
		}
		if left.Branch > right.Branch {
			return 1
		}
		return 0
	})
	if !slices.Equal(members, want) {
		t.Fatalf("mapped canonical refs=%v, want %v", members, want)
	}
	if scratch.groupsRetainedBytes == 0 || scratch.groupsBucketCount == 0 {
		t.Fatalf("mapped canonical groups were not accounted: %+v", scratch)
	}
	mismatched := []diagnosticParserCoreHeader{
		{head: heads[0], frontierSequence: 31},
		{head: heads[0], frontierSequence: 32},
	}
	if out, err := scratch.canonicalize(compact, mismatched); err != nil || len(out) != 1 || out[0].frontierSequence != 0 {
		t.Fatalf("mismatched mapped frontier=%+v err=%v, want one cleared sequence", out, err)
	}
	peak := scratch.groupsRetainedBytes
	_, err = scratch.canonicalize(compact, headers[:1])
	if err != nil {
		t.Fatal(err)
	}
	if scratch.groupsRetainedBytes != peak {
		t.Fatalf("canonical group clear dropped retained peak: got %d want %d", scratch.groupsRetainedBytes, peak)
	}

	scheduler := &diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{{head: head, dropCohortRefs: refs}}}
	candidates := scheduler.collectCondenseCandidates(-1)
	if len(candidates) != 1 || !slices.Equal(g18RootMembers(t, compact, candidates[0].DropCohortRefs), g18RootMembers(t, compact, refs)) {
		t.Fatalf("condense candidates=%+v", candidates)
	}
}

func TestG18AuthenticGenericReductionEstablishesProducerCohort(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 2}: 4,
			{state: 2, symbol: 2}: 4,
		},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondSeed, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	token := core.Token{Symbol: 8, StartByte: 0, EndByte: 1}
	_, err = compact.Shift(seed, 8, 0, token, core.ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Shift(secondSeed, 8, 0, token, core.ForkOrder{Present: true, Value: 2})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := compact.Derivations(second)
	if err != nil || len(paths) != 2 {
		t.Fatalf("multi-path setup derivations=%d err=%v", len(paths), err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: second, creationSeq: 1}},
		token: Token{
			Symbol: 9, StartByte: 1, EndByte: 2,
			StartPoint: Point{Row: 4, Column: 6}, EndPoint: Point{Row: 4, Column: 7},
		},
		nextCleanPathLineage: 1,
		options:              DiagnosticParserCorePrefixOptions{MaxDispatches: 20, recordDropCohortCertificates: true}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].dropCohortRefs.Len() != 1 {
		t.Fatalf("producer header=%+v, want one branch reference", scheduler.headers)
	}
	ref, ok := compact.DropCohortRefAt(scheduler.headers[0].dropCohortRefs, 0)
	if !ok {
		t.Fatal("producer header reference is unreadable")
	}
	state, expected, written, err := compact.DropCohortState(core.DropCohortHandle{Owner: ref.Owner, Epoch: ref.Epoch, Sequence: ref.Sequence})
	if err != nil || state != core.DropCohortComplete || expected != 1 || written != 1 {
		t.Fatalf("producer state=%v expected=%d written=%d err=%v", state, expected, written, err)
	}
	storedAction, ok := compact.DropCohortAction(core.DropCohortHandle{Owner: ref.Owner, Epoch: ref.Epoch, Sequence: ref.Sequence})
	if !ok || storedAction.BoundaryState != 3 || storedAction.Lookahead != 9 || storedAction.ActionOrdinal != 0 ||
		storedAction.Action.Type != core.ActionReduce || storedAction.Action.Symbol != 2 || storedAction.NoLookahead ||
		storedAction.Selection != core.DropCohortSelectionNone {
		t.Fatalf("producer action identity=%+v, want state=3 lookahead=9 ordinal=0 reduce selection=none", storedAction)
	}
	counters := compact.DropCohortProducerCounts()
	if counters.ReductionEstablishment != 1 {
		t.Fatalf("authentic producer counters=%+v, want one establishment", counters)
	}
}

func TestG18GenericReductionLeavesDropCohortsInertByDefault(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 2}: 4,
			{state: 2, symbol: 2}: 4,
		},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondSeed, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	token := core.Token{Symbol: 8, StartByte: 0, EndByte: 1}
	if _, err = compact.Shift(seed, 8, 0, token, core.ForkOrder{Present: true, Value: 1}); err != nil {
		t.Fatal(err)
	}
	second, err := compact.Shift(secondSeed, 8, 0, token, core.ForkOrder{Present: true, Value: 2})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := compact.Derivations(second)
	if err != nil || len(paths) != 2 {
		t.Fatalf("multi-path setup derivations=%d err=%v", len(paths), err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: second, creationSeq: 1}},
		token: Token{
			Symbol: 9, StartByte: 1, EndByte: 2,
			StartPoint: Point{Row: 4, Column: 6}, EndPoint: Point{Row: 4, Column: 7},
		},
		nextCleanPathLineage: 1,
		options:              DiagnosticParserCorePrefixOptions{MaxDispatches: 20},
		receipt:              &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].dropCohortRefs.Len() != 0 {
		t.Fatalf("default-off producer header=%+v, want one branch without cohort refs", scheduler.headers)
	}
	if scheduler.headers[0].cleanPathLineage == 0 || scheduler.headers[0].altSet.Len() == 0 || !scheduler.headers[0].convergedReductionSplit {
		t.Fatalf("default-off lineage state=%+v, want retained lineage and alternative set", scheduler.headers[0])
	}
	counters := compact.DropCohortProducerCounts()
	// RecordReductionLineageOwned remains unconditional. Its one authentic
	// lineage establishment is distinct from the gated cohort construction.
	if counters.ReductionEstablishment != 1 {
		t.Fatalf("default-off producer counters=%+v, want one lineage establishment", counters)
	}
}

func TestG18RefSetCanonicalGroupsBucketGrowthBoundaries(t *testing.T) {
	previous := uint64(0)
	for _, entries := range []int{1, 6, 7, 13, 14, 26, 27} {
		buckets, bytes := diagnosticParserCoreCanonicalGroupsBucketBytes(entries)
		if buckets == 0 || bytes < previous {
			t.Fatalf("entries=%d buckets=%d bytes=%d previous=%d", entries, buckets, bytes, previous)
		}
		previous = bytes
	}
}

func TestG18RefSetCanonicalGroupsDuplicateHintRetention(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	refs := g18RootRefs(t, compact)
	normalized := make([]diagnosticParserCoreHeader, 0, 33)
	for index := 0; index < cap(normalized); index++ {
		normalized = append(normalized, diagnosticParserCoreHeader{head: head, creationSeq: uint64(index + 1), dropCohortRefs: refs})
	}
	var scratch diagnosticParserCoreCanonicalScratch
	if _, err := scratch.canonicalize(compact, normalized); err != nil {
		t.Fatal(err)
	}
	_, hinted := diagnosticParserCoreCanonicalGroupsBucketBytes(len(normalized))
	if scratch.groupsRetainedBytes < hinted {
		t.Fatalf("duplicate-heavy map hint under-accounted: got %d want at least %d", scratch.groupsRetainedBytes, hinted)
	}
	peak := scratch.groupsRetainedBytes
	if _, err := scratch.canonicalize(compact, normalized[:1]); err != nil {
		t.Fatal(err)
	}
	if scratch.groupsRetainedBytes != peak {
		t.Fatalf("clear/reuse dropped hinted peak: got %d want %d", scratch.groupsRetainedBytes, peak)
	}
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, canonicalScratch: scratch}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatal(err)
	}
	if scheduler.canonicalScratch.groupsRetainedBytes != 0 || scheduler.canonicalScratch.groups != nil {
		t.Fatalf("reset retained canonical map estimate: %+v", scheduler.canonicalScratch)
	}
}

func TestG18AuthenticProducerCanonicalizationCounters(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact}
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.headers = []diagnosticParserCoreHeader{
			{head: head, creationSeq: 1}, {head: head, creationSeq: 2},
		}
		if err := scheduler.canonicalizeOwned(owner); err != nil {
			return err
		}
		scheduler.headers = make([]diagnosticParserCoreHeader, 9)
		for index := range scheduler.headers {
			scheduler.headers[index] = diagnosticParserCoreHeader{head: head, creationSeq: uint64(index + 1)}
		}
		return scheduler.canonicalizeOwned(owner)
	}); err != nil {
		t.Fatal(err)
	}
	counters := compact.DropCohortProducerCounts()
	if counters.LinearCanonicalizer != 1 || counters.MappedCanonicalizer != 1 {
		t.Fatalf("authentic canonicalizer counters=%+v, want linear=1 mapped=1", counters)
	}
}

func TestG18RefSetHeaderPersistenceSiblingAdoptionAndConflictReconciliation(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	refs := g18RootRefs(t, compact)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: head, creationSeq: 1, dropCohortRefs: refs, convergedReductionSplit: true, frontierSequence: 41}},
	}
	if err := compact.RunFreshSchedulerSession(func(owner core.SchedulerTransactionToken) error {
		return scheduler.persistHeaderLineageOwned(owner)
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := compact.NodeLineageDropCohortRefs(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g18RootMembers(t, compact, persisted), g18RootMembers(t, compact, refs)) {
		t.Fatalf("persisted refs=%v, want %v", persisted, refs)
	}

	secondary := diagnosticParserCoreHeader{head: head, creationSeq: 2}
	scheduler.headers = []diagnosticParserCoreHeader{scheduler.headers[0], secondary}
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		adopted, err := scheduler.adoptUpdatedReductionSiblingOwned(owner, 0, head, core.CleanPathRankUnknown, 0, core.AlternativeSet{}, false, refs, false, false, core.DropCohortProducerSiblingAdoption)
		if err != nil || !adopted {
			return fmt.Errorf("sibling adoption adopted=%t err=%v", adopted, err)
		}
		if !slices.Equal(g18RootMembers(t, compact, scheduler.headers[1].dropCohortRefs), g18RootMembers(t, compact, refs)) {
			return fmt.Errorf("adopted sibling refs=%+v", scheduler.headers[1].dropCohortRefs)
		}
		if scheduler.headers[1].frontierSequence != 41 {
			return fmt.Errorf("adopted sibling frontier=%d, want 41", scheduler.headers[1].frontierSequence)
		}

		scheduler.headers = []diagnosticParserCoreHeader{{head: head, creationSeq: 1, frontierSequence: 43}, {head: head, creationSeq: 2}}
		outputs, conflictAdopted, err := scheduler.reconcileGenericConflictOutputs(owner, 0, []diagnosticParserCoreHeader{{
			head: head, freshness: core.ReductionUpdated, dropCohortRefs: refs,
		}})
		if err != nil || conflictAdopted != 1 || len(outputs) != 0 {
			return fmt.Errorf("conflict reconciliation outputs=%+v adopted=%d err=%v", outputs, conflictAdopted, err)
		}
		if !slices.Equal(g18RootMembers(t, compact, scheduler.headers[1].dropCohortRefs), g18RootMembers(t, compact, refs)) {
			return fmt.Errorf("reconciled sibling refs=%+v", scheduler.headers[1].dropCohortRefs)
		}
		if scheduler.headers[1].frontierSequence != 43 {
			return fmt.Errorf("reconciled sibling frontier=%d, want 43", scheduler.headers[1].frontierSequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	counters := compact.DropCohortProducerCounts()
	if counters.SiblingAdoption != 1 || counters.ConflictReconciliation != 1 {
		t.Fatalf("authentic sibling/conflict counters=%+v, want sibling=1 conflict=1", counters)
	}
}

func TestG18RefSetPoolClearing(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	refs := g18RootRefs(t, compact)

	headerScratch := make([]diagnosticParserCoreHeader, 1, 1)
	headerScratch[0].dropCohortRefs = refs
	reductionScratch := make([]core.ReductionOutput, 1, 1)
	reductionScratch[0].DropCohortRefs = refs
	candidateScratch := make([]core.CondenseCandidate, 1, 1)
	candidateScratch[0] = core.CondenseCandidate{Head: head, DropCohortRefs: refs}
	canonicalHeaders := make([]diagnosticParserCoreHeader, 1, 1)
	canonicalHeaders[0].dropCohortRefs = refs
	canonical := diagnosticParserCoreCanonicalScratch{
		headerBuffers: [2][]diagnosticParserCoreHeader{canonicalHeaders, canonicalHeaders},
		keys:          make([]diagnosticParserCorePhaseHead, 1, 1),
		groups:        map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup{{}: {dropCohortRefs: refs}},
	}
	canonical.inlineHeaders[0][0].dropCohortRefs = refs

	scheduler := diagnosticParserCoreGenericScheduler{
		canonicalScratch:      canonical,
		reductionReplacements: headerScratch,
		reductionOutputs:      reductionScratch,
		condenseCandidates:    candidateScratch,
	}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatal(err)
	}
	if got := scheduler.reductionReplacements[:cap(scheduler.reductionReplacements)][0].dropCohortRefs; got != (core.DropCohortRefSet{}) {
		t.Fatalf("header scratch retained refs=%+v", got)
	}
	if got := scheduler.reductionOutputs[:cap(scheduler.reductionOutputs)][0].DropCohortRefs; got != (core.DropCohortRefSet{}) {
		t.Fatalf("reduction scratch retained refs=%+v", got)
	}
	if got := scheduler.condenseCandidates[:cap(scheduler.condenseCandidates)][0].DropCohortRefs; got != (core.DropCohortRefSet{}) {
		t.Fatalf("candidate scratch retained refs=%+v", got)
	}
	for index, buffer := range scheduler.canonicalScratch.headerBuffers {
		if buffer != nil {
			t.Fatalf("canonical header buffer %d retained backing storage", index)
		}
	}
	if got := scheduler.canonicalScratch.inlineHeaders[0][0].dropCohortRefs; got != (core.DropCohortRefSet{}) {
		t.Fatalf("inline canonical headers retained refs=%+v", got)
	}
	if scheduler.canonicalScratch.groups != nil {
		t.Fatalf("canonical groups retained backing storage")
	}
}

func TestG18RefSetSchedulerMemoryBudgetAccounting(t *testing.T) {
	compact, _, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact}
	base := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
	scheduler.headers = make([]diagnosticParserCoreHeader, 0, 2)
	scheduler.summaryHeaderScratch = make([]DiagnosticParserCoreHeaderReceipt, 0, 21)
	scheduler.headerRollbackScratch.headers = make([]diagnosticParserCoreHeader, 0, 3)
	scheduler.canonicalScratch.headerBuffers[0] = make([]diagnosticParserCoreHeader, 0, 4)
	scheduler.canonicalScratch.keys = make([]diagnosticParserCorePhaseHead, 0, 5)
	scheduler.canonicalScratch.groups = make(map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup)
	for index := 0; index < 6; index++ {
		scheduler.canonicalScratch.groups[diagnosticParserCorePhaseHead{head: core.Head{Node: core.NodeID(index + 1)}}] = diagnosticParserCoreCanonicalGroup{}
	}
	scheduler.canonicalScratch.observeGroupsInsertion(6)
	scheduler.dispatchScratch.cells = make([]diagnosticParserCoreGenericCell, 0, 6)
	scheduler.dispatchScratch.noActionIndices = make([]int, 0, 7)
	scheduler.conflictScratch.actionOutputs = make([]diagnosticParserCoreActionOutput, 0, 8)
	scheduler.conflictScratch.reductionOutputs = make([]core.ReductionOutput, 0, 9)
	scheduler.conflictScratch.outputs = make([]diagnosticParserCoreHeader, 0, 10)
	scheduler.conflictScratch.armRanges = make([]diagnosticParserCoreConflictArmRange, 0, 11)
	scheduler.conflictScratch.adopted = make([]int, 0, 12)
	scheduler.conflictScratch.headerAssembly = make([]diagnosticParserCoreHeader, 0, 13)
	scheduler.reductionOutputs = make([]core.ReductionOutput, 0, 14)
	scheduler.reductionReplacements = make([]diagnosticParserCoreHeader, 0, 15)
	scheduler.classifiedBoundaries = make([]core.ClassifiedBoundary, 0, 16)
	scheduler.condenseCandidates = make([]core.CondenseCandidate, 0, 17)
	scheduler.electStates = make([]StateID, 0, 18)
	scheduler.electGLRStates = make([]StateID, 0, 19)
	scheduler.acceptedPayloads = make([]core.SubtreeID, 0, 20)
	got := diagnosticParserCoreSchedulerFootprintBytes(&scheduler) - base
	want := uint64(2)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(21)*uint64(unsafe.Sizeof(DiagnosticParserCoreHeaderReceipt{})) +
		uint64(3)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(4)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(5)*uint64(unsafe.Sizeof(diagnosticParserCorePhaseHead{})) +
		scheduler.canonicalScratch.groupsRetainedBytes +
		uint64(6)*uint64(unsafe.Sizeof(diagnosticParserCoreGenericCell{})) +
		uint64(7)*uint64(unsafe.Sizeof(int(0))) +
		uint64(8)*uint64(unsafe.Sizeof(diagnosticParserCoreActionOutput{})) +
		uint64(9)*uint64(unsafe.Sizeof(core.ReductionOutput{})) +
		uint64(10)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(11)*uint64(unsafe.Sizeof(diagnosticParserCoreConflictArmRange{})) +
		uint64(12)*uint64(unsafe.Sizeof(int(0))) +
		uint64(13)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(14)*uint64(unsafe.Sizeof(core.ReductionOutput{})) +
		uint64(15)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(16)*uint64(unsafe.Sizeof(core.ClassifiedBoundary{})) +
		uint64(17)*uint64(unsafe.Sizeof(core.CondenseCandidate{})) +
		uint64(18)*uint64(unsafe.Sizeof(StateID(0))) +
		uint64(19)*uint64(unsafe.Sizeof(StateID(0))) +
		uint64(20)*uint64(unsafe.Sizeof(core.SubtreeID(0)))
	if got != want {
		t.Fatalf("scheduler footprint delta=%d, want %d", got, want)
	}
	scheduler.options.stopControlMemoryBudgetBytes = int64(diagnosticParserCoreSchedulerFootprintBytes(&scheduler))
	if reason := scheduler.stopControlMemoryBudgetReason(); reason != ParseStopMemoryBudget {
		t.Fatalf("budget stop reason=%v, want memory budget", reason)
	}
	scheduler.options.stopControlMemoryBudgetBytes++
	if reason := scheduler.stopControlMemoryBudgetReason(); reason != ParseStopNone {
		t.Fatalf("budget-plus-one reason=%v, want none", reason)
	}
}

func TestG18RefSetSchedulerRecordSizeRatchets(t *testing.T) {
	if got := unsafe.Sizeof(diagnosticParserCoreHeader{}); got != 224 {
		t.Fatalf("scheduler header size=%d, want 224", got)
	}
	if got := unsafe.Sizeof(diagnosticParserCoreCanonicalGroup{}); got != 104 {
		t.Fatalf("canonical group size=%d, want 104", got)
	}
	if got := unsafe.Sizeof(diagnosticParserCoreActionOutput{}); got != 104 {
		t.Fatalf("action output size=%d, want 104", got)
	}
	if got := unsafe.Sizeof(core.CondenseCandidate{}); got != 88 {
		t.Fatalf("condense candidate size=%d, want 88", got)
	}
}

func TestG18DropCohortFrontierDefaultOffHasNoAllocationOrStorage(t *testing.T) {
	compact, _, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	scheduler := &diagnosticParserCoreGenericScheduler{compact: compact}
	baseSchedulerFootprint := diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	baseStorage := compact.StorageBytes()
	allocs := testing.AllocsPerRun(100, func() {
		if err := scheduler.publishDropCohortFrontierOwned(); err != nil {
			t.Fatalf("default-off producer returned error: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("default-off frontier producer allocations=%f, want 0", allocs)
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(scheduler); got != baseSchedulerFootprint {
		t.Fatalf("default-off scheduler footprint=%d, want unchanged %d", got, baseSchedulerFootprint)
	}
	if got := compact.StorageBytes(); got != baseStorage {
		t.Fatalf("default-off compact storage=%d, want unchanged %d", got, baseStorage)
	}
	if scheduler.options.recordDropCohortCertificates || scheduler.canonicalScratch.groups != nil {
		t.Fatalf("default-off scheduler state expanded: options=%+v canonical=%+v", scheduler.options, scheduler.canonicalScratch)
	}
	t.Logf("default-off scheduler=%d bytes, header=%d bytes, compact storage=%d bytes", unsafe.Sizeof(*scheduler), unsafe.Sizeof(diagnosticParserCoreHeader{}), baseStorage)
}

func TestG18FrontierHeaderReplacementAndRollbackPropagation(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	headers := []diagnosticParserCoreHeader{{head: head, frontierSequence: 51}}
	replaced := replaceDiagnosticParserCoreHeader(headers, 0, []diagnosticParserCoreHeader{{head: head, frontierSequence: 52}})
	if len(replaced) != 1 || replaced[0].frontierSequence != 52 {
		t.Fatalf("replacement frontier=%+v, want 52", replaced)
	}
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: replaced}
	if err := scheduler.headerRollbackScratch.begin(scheduler.headers); err != nil {
		t.Fatal(err)
	}
	scheduler.headers[0].frontierSequence = 99
	scheduler.headerRollbackScratch.finish(&scheduler.headers, true)
	if scheduler.headers[0].frontierSequence != 52 {
		t.Fatalf("rollback frontier=%d, want 52", scheduler.headers[0].frontierSequence)
	}
}
