//go:build gts_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreCanonicalVersionLexerSnapshotIdentity(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	for _, count := range []int{2, diagnosticParserCoreLinearCanonicalLimit + 1} {
		t.Run("frontier_"+string(rune('0'+count)), func(t *testing.T) {
			firstSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
			secondSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
			first := &diagnosticParserCoreVersionState{relexSnapshot: firstSnapshot}
			second := &diagnosticParserCoreVersionState{relexSnapshot: secondSnapshot}
			headers := make([]diagnosticParserCoreHeader, count)
			for index := range headers {
				headers[index] = diagnosticParserCoreHeader{head: head, versionState: first}
			}
			headers[count-1].versionState = second
			var scratch diagnosticParserCoreCanonicalScratch
			out, err := scratch.canonicalize(compact, headers)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 2 {
				t.Fatalf("divergent lexer snapshots merged: len=%d output=%+v", len(out), out)
			}
			seenFirst, seenSecond := false, false
			for _, header := range out {
				seenFirst = seenFirst || header.versionState == first
				seenSecond = seenSecond || header.versionState == second
			}
			if !seenFirst || !seenSecond {
				t.Fatalf("canonical output lost a lexer snapshot identity: %+v", out)
			}

			shared := make([]diagnosticParserCoreHeader, count)
			for index := range shared {
				shared[index] = diagnosticParserCoreHeader{head: head, versionState: first}
			}
			out, err = scratch.canonicalize(compact, shared)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 1 || out[0].versionState != first {
				t.Fatalf("equal lexer snapshots did not merge: len=%d output=%+v", len(out), out)
			}

			shared[count-1].versionState = &diagnosticParserCoreVersionState{
				s3Region: &diagnosticParserCoreS3Region{}, relexSnapshot: firstSnapshot,
			}
			out, err = scratch.canonicalize(compact, shared)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 2 {
				t.Fatalf("distinct recovery region merged through equal lexer snapshots: len=%d output=%+v", len(out), out)
			}
		})
	}
}

func TestDiagnosticParserCoreReductionSiblingAdoptionRequiresVersionLexerSnapshotIdentity(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	firstSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	secondSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	first := &diagnosticParserCoreVersionState{relexSnapshot: firstSnapshot}
	second := &diagnosticParserCoreVersionState{relexSnapshot: secondSnapshot}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: head, versionState: first},
			{head: head, versionState: second, paused: true},
		},
	}
	adopted, err := scheduler.adoptUpdatedReductionSibling(
		0, head, core.CleanPathRankUnknown, 0, core.AlternativeSet{}, false, false, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if adopted {
		t.Fatal("sibling adoption merged divergent lexer snapshots")
	}
	if scheduler.headers[1].versionState != second || !scheduler.headers[1].paused {
		t.Fatalf("divergent sibling changed: %+v", scheduler.headers[1])
	}

	scheduler.headers[1].versionState = &diagnosticParserCoreVersionState{
		s3Region: &diagnosticParserCoreS3Region{}, relexSnapshot: firstSnapshot,
	}
	adopted, err = scheduler.adoptUpdatedReductionSibling(
		0, head, core.CleanPathRankUnknown, 0, core.AlternativeSet{}, false, false, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if adopted {
		t.Fatal("sibling adoption merged a distinct recovery region")
	}
	scheduler.headers[1].versionState = first
	adopted, err = scheduler.adoptUpdatedReductionSibling(
		0, head, core.CleanPathRankUnknown, 0, core.AlternativeSet{}, false, false, false,
	)
	if err != nil || !adopted {
		t.Fatalf("equal immutable version state was not adopted: adopted=%t err=%v", adopted, err)
	}
}

func TestDiagnosticParserCoreReductionUnchangedDistinctLexerSnapshotIsRetained(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	first := &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}
	second := &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: head, versionState: first},
			{head: head, versionState: second},
		},
	}
	kept, adopted, err := scheduler.reconcileGenericConflictOutputs(0, []diagnosticParserCoreHeader{
		{head: head, versionState: second, freshness: core.ReductionUnchanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	if adopted != 0 || len(kept) != 1 || kept[0].versionState != second || kept[0].freshness != core.ReductionUnchanged {
		t.Fatalf("distinct unchanged output was dropped: kept=%+v adopted=%d", kept, adopted)
	}
}

func TestDiagnosticParserCoreGenericReductionUnchangedDistinctLexerSnapshotFailsClosedBeforePhysicalMerge(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, source := newGenericFreshnessSource(t, table)
	initial, err := compact.ReduceOutputs(source, 9, 0, core.ForkOrder{})
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial reduction outputs=%+v err=%v", initial, err)
	}
	if initial[0].Freshness != core.ReductionNew {
		t.Fatalf("initial reduction freshness=%v, want new prepopulation", initial[0].Freshness)
	}
	first := &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}
	second := &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: source, creationSeq: 6, versionState: first},
			{head: initial[0].Head, creationSeq: 7, versionState: second},
		},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 20,
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	err = scheduler.applyGenericReduction(before, cell)
	if err == nil || !strings.Contains(err.Error(), "compact head has multiple scheduler owners") {
		t.Fatalf("distinct-snapshot physical alias error=%v, want explicit owner conflict", err)
	}
	if len(scheduler.reductionOutputs) != 1 || scheduler.reductionOutputs[0].Freshness != core.ReductionUnchanged {
		t.Fatalf("repeated reduction outputs=%+v, want one unchanged output", scheduler.reductionOutputs)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("distinct unchanged reduction output changed the frontier before failing closed: headers=%+v", scheduler.headers)
	}
	seenFirst, seenSecond := false, false
	for _, header := range scheduler.headers {
		seenFirst = seenFirst || header.versionState == first
		seenSecond = seenSecond || header.versionState == second
	}
	if !seenFirst || !seenSecond {
		t.Fatalf("reduction retained the wrong version identities: %+v", scheduler.headers)
	}
	if scheduler.headers[0].head != source || scheduler.headers[1].head != initial[0].Head {
		t.Fatalf("owner-conflict rollback did not restore the original physical heads: headers=%+v", scheduler.headers)
	}
}

func TestDiagnosticParserCoreGenericReductionUnchangedEqualSnapshotDoesNotDuplicatePhysicalHead(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, source := newGenericFreshnessSource(t, table)
	initial, err := compact.ReduceOutputs(source, 9, 0, core.ForkOrder{})
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial reduction outputs=%+v err=%v", initial, err)
	}
	if initial[0].Freshness != core.ReductionNew {
		t.Fatalf("initial reduction freshness=%v, want new prepopulation", initial[0].Freshness)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: source, creationSeq: 6},
			{head: initial[0].Head, creationSeq: 7},
		},
		token: Token{Symbol: 9, StartByte: 1, EndByte: 2},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 20,
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, scheduler.headers[0], 9)
	if err := scheduler.applyGenericReduction(before, cell); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.reductionOutputs) != 1 || scheduler.reductionOutputs[0].Freshness != core.ReductionUnchanged {
		t.Fatalf("repeated reduction outputs=%+v, want one unchanged output", scheduler.reductionOutputs)
	}
	if len(scheduler.headers) != 2 || scheduler.headers[0].head != source || !scheduler.headers[0].paused ||
		scheduler.headers[1].head != initial[0].Head || scheduler.headers[1].paused {
		t.Fatalf("equal-snapshot unchanged output duplicated its physical sibling: %+v", scheduler.headers)
	}
	if scheduler.headers[0].head.Node == scheduler.headers[1].head.Node {
		t.Fatalf("equal-snapshot unchanged output retained duplicate physical node %d", scheduler.headers[0].head.Node)
	}
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerOwned(owner, scheduler.headers[1].head, 9)
	}); err == nil || !strings.Contains(err.Error(), "multiple scheduler owners") {
		t.Fatalf("scheduler persistence did not bind the surviving physical head to owner 8: %v", err)
	}
	if err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return compact.RecordHeadOwnerOwned(owner, scheduler.headers[1].head, 8)
	}); err != nil {
		t.Fatalf("surviving physical head rejected its persisted scheduler owner 8: %v", err)
	}
}

func TestDiagnosticParserCoreCanonicalClearsDiscardedVersionStateTail(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	state := &diagnosticParserCoreVersionState{}
	input := make([]diagnosticParserCoreHeader, 3, 4)
	for index := range input {
		input[index] = diagnosticParserCoreHeader{head: head, versionState: state}
	}
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || cap(out) <= len(out) {
		t.Fatalf("canonical output len/cap=%d/%d, want a retained tail", len(out), cap(out))
	}
	for index, header := range out[:cap(out)][len(out):] {
		if header.versionState != nil {
			t.Fatalf("canonical discarded slot %d retained version state", index+len(out))
		}
	}
}

func TestDiagnosticParserCoreRollbackClearsDiscardedVersionStateTail(t *testing.T) {
	keep := &diagnosticParserCoreVersionState{}
	discarded := &diagnosticParserCoreVersionState{}
	headers := make([]diagnosticParserCoreHeader, 1, 4)
	headers[0].versionState = keep
	var scratch diagnosticParserCoreHeaderRollbackScratch
	if err := scratch.begin(headers); err != nil {
		t.Fatal(err)
	}
	headers = append(headers, diagnosticParserCoreHeader{versionState: discarded})
	scratch.finish(&headers, true)
	if len(headers) != 1 || headers[0].versionState != keep {
		t.Fatalf("rollback did not restore the original header: %+v", headers)
	}
	if headers[:cap(headers)][1].versionState != nil {
		t.Fatal("rollback retained a discarded header state beyond len")
	}
	if scratch.inline[0].versionState != nil {
		t.Fatal("rollback scratch retained its snapshot state")
	}
}

func TestDiagnosticParserCoreConflictScratchClearsDiscardedVersionStateTail(t *testing.T) {
	state := &diagnosticParserCoreVersionState{}
	scratch := diagnosticParserCoreConflictScratch{
		outputs:        make([]diagnosticParserCoreHeader, 3, 5),
		headerAssembly: make([]diagnosticParserCoreHeader, 3, 5),
	}
	for index := range scratch.outputs {
		scratch.outputs[index].versionState = state
		scratch.headerAssembly[index].versionState = state
	}
	if err := scratch.begin(1); err != nil {
		t.Fatal(err)
	}
	for name, headers := range map[string][]diagnosticParserCoreHeader{
		"outputs":  scratch.outputs,
		"assembly": scratch.headerAssembly,
	} {
		if len(headers) != 0 || cap(headers) <= len(headers) {
			t.Fatalf("%s len/cap=%d/%d, want an empty retained buffer", name, len(headers), cap(headers))
		}
		for index, header := range headers[:cap(headers)] {
			if header.versionState != nil {
				t.Fatalf("%s discarded slot %d retained version state", name, index)
			}
		}
	}
	scratch.finish()
}

func TestDiagnosticParserCoreReductionReplacementResetClearsStaleTail(t *testing.T) {
	state := &diagnosticParserCoreVersionState{}
	replacements := make([]diagnosticParserCoreHeader, 1, 4)
	replacements[0].versionState = state
	replacements[:cap(replacements)][1].versionState = state
	scheduler := diagnosticParserCoreGenericScheduler{reductionReplacements: replacements}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatal(err)
	}
	if replacements[:cap(replacements)][0].versionState != nil || replacements[:cap(replacements)][1].versionState != nil {
		t.Fatal("reduction replacement reset retained version state")
	}
}

func TestDiagnosticParserCoreNoActionClearsDiscardedVersionStateTail(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	keep := &diagnosticParserCoreVersionState{}
	discarded := &diagnosticParserCoreVersionState{}
	headers := make([]diagnosticParserCoreHeader, 2, 4)
	headers[0] = diagnosticParserCoreHeader{head: head, shifted: true, versionState: keep}
	headers[1] = diagnosticParserCoreHeader{head: head, paused: true, versionState: discarded}
	scheduler := diagnosticParserCoreGenericScheduler{
		compact:           compact,
		headers:           headers,
		epochProgress:     true,
		verifierBound:     len(headers),
		verifierHeaderPtr: &headers[0],
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:                     DiagnosticParserCoreReceiptSummary,
			allowConvergedSplitDropArtifact: true,
		},
	}
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].versionState != keep {
		t.Fatalf("no-action drop changed the survivor: %+v", scheduler.headers)
	}
	if headers[:cap(headers)][1].versionState != nil {
		t.Fatal("no-action drop retained the discarded header state")
	}
	if scheduler.verifierHeaderPtr != nil || scheduler.verifierBound != 0 {
		t.Fatal("no-action drop retained a stale verifier binding")
	}
}

func TestDiagnosticParserCoreRecoveryCollapseClearsDiscardedVersionStateTail(t *testing.T) {
	keep := &diagnosticParserCoreVersionState{
		relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}, lexerRequest: 3,
		acceptanceSeq: 7, recoveryGroup: 5, missingGroup: 6,
		recoveryNodeBaseline: 8, recoveryNodeBaselineSet: true,
	}
	before := *keep
	want := before
	want.recoveryGroup, want.missingGroup = 0, 0
	want.recoveryNodeBaseline, want.recoveryNodeBaselineSet = 0, false
	discarded := &diagnosticParserCoreVersionState{}
	active := make([]diagnosticParserCoreHeader, 2, 4)
	active[0].versionState = keep
	active[1].versionState = discarded
	canonical := make([]diagnosticParserCoreHeader, 2, 4)
	canonical[0].versionState = keep
	canonical[1].versionState = discarded
	scheduler := diagnosticParserCoreGenericScheduler{
		headers: active,
		canonicalScratch: diagnosticParserCoreCanonicalScratch{
			headerBuffers: [2][]diagnosticParserCoreHeader{canonical, canonical},
			keys: []diagnosticParserCorePhaseHead{
				{versionState: discarded},
			},
			groups: map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup{
				{versionState: discarded}: {},
			},
		},
	}
	scheduler.collapseToRecoveryWinner(0)
	if len(scheduler.headers) != 1 || scheduler.headers[0].versionState == nil || *scheduler.headers[0].versionState != want {
		t.Fatalf("recovery collapse changed the winner: %+v", scheduler.headers)
	}
	if *keep != before {
		t.Fatal("recovery collapse mutated the published winner state")
	}
	if active[:cap(active)][1].versionState != nil {
		t.Fatal("recovery collapse retained a discarded active state")
	}
	for index, buffer := range scheduler.canonicalScratch.headerBuffers {
		if buffer[:cap(buffer)][0].versionState != nil || buffer[:cap(buffer)][1].versionState != nil {
			t.Fatalf("recovery collapse retained canonical buffer %d state", index)
		}
	}
	if len(scheduler.canonicalScratch.keys) != 0 || len(scheduler.canonicalScratch.groups) != 0 {
		t.Fatalf("recovery collapse retained canonical keys/groups=%d/%d",
			len(scheduler.canonicalScratch.keys), len(scheduler.canonicalScratch.groups))
	}
}

func TestDiagnosticParserCoreRecoveryCollapseReleasesEmptyWinnerState(t *testing.T) {
	scheduler := diagnosticParserCoreGenericScheduler{
		headers: []diagnosticParserCoreHeader{{versionState: &diagnosticParserCoreVersionState{}}},
	}
	scheduler.collapseToRecoveryWinner(0)
	if len(scheduler.headers) != 1 || scheduler.headers[0].versionState != nil {
		t.Fatal("recovery collapse retained an empty winner state")
	}
}

func TestDiagnosticParserCoreMappedCanonicalErrorReleasesVersionStateKeys(t *testing.T) {
	compact, _, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	headers := make([]diagnosticParserCoreHeader, 0, diagnosticParserCoreLinearCanonicalLimit+2)
	for index := 0; index <= diagnosticParserCoreLinearCanonicalLimit; index++ {
		head, err := compact.Seed(core.StateID(index+10), 0)
		if err != nil {
			t.Fatal(err)
		}
		headers = append(headers, diagnosticParserCoreHeader{
			head: head, versionState: &diagnosticParserCoreVersionState{},
		})
	}
	malformed := headers[0]
	malformed.dropCohortRefs = core.DropCohortRefSet{Count: 3}
	headers = append(headers, malformed)
	var scratch diagnosticParserCoreCanonicalScratch
	if _, err := scratch.canonicalize(compact, headers); err == nil {
		t.Fatal("mapped canonicalization accepted an invalid reference set")
	}
	if len(scratch.keys) != 0 || len(scratch.groups) != 0 {
		t.Fatalf("mapped canonical error retained keys/groups=%d/%d", len(scratch.keys), len(scratch.groups))
	}
}

func TestDiagnosticParserCoreReplacementClearsDiscardedVersionStateTail(t *testing.T) {
	state := &diagnosticParserCoreVersionState{}
	headers := make([]diagnosticParserCoreHeader, 3, 5)
	for index := range headers {
		headers[index].versionState = state
	}
	replacements := []diagnosticParserCoreHeader{}
	got := replaceDiagnosticParserCoreHeader(headers, 1, replacements)
	if len(got) != 2 || cap(got) != cap(headers) {
		t.Fatalf("replacement unexpectedly changed storage: len/cap=%d/%d", len(got), cap(got))
	}
	if got[:cap(got)][2].versionState != nil {
		t.Fatal("replacement retained its discarded tail state")
	}
}

func TestDiagnosticParserCoreSchedulerFootprintCountsWideSharedVersionStates(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	const totalHeaders = 300
	const uniqueStates = 257
	states := make([]*diagnosticParserCoreVersionState, uniqueStates)
	for index := range states {
		states[index] = &diagnosticParserCoreVersionState{}
	}
	headers := make([]diagnosticParserCoreHeader, totalHeaders)
	for index := range headers {
		stateIndex := index
		if stateIndex >= uniqueStates {
			stateIndex = uniqueStates - 1
		}
		headers[index] = diagnosticParserCoreHeader{head: head, versionState: states[stateIndex]}
	}
	base := diagnosticParserCoreSchedulerFootprintBytes(&diagnosticParserCoreGenericScheduler{compact: compact})
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: headers,
	}
	got := diagnosticParserCoreSchedulerFootprintBytes(scheduler) - base
	want := uint64(totalHeaders)*uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})) +
		uint64(uniqueStates)*uint64(unsafe.Sizeof(diagnosticParserCoreVersionState{})) +
		uint64(cap(scheduler.footprintRefs))*uint64(unsafe.Sizeof(diagnosticParserCoreFootprintRef{}))
	if got != want {
		t.Fatalf("wide shared-state footprint=%d, want exact %d", got, want)
	}
	var gotAgain uint64
	if allocs := testing.AllocsPerRun(100, func() {
		gotAgain = diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	}); allocs != 0 || gotAgain != diagnosticParserCoreSchedulerFootprintBytes(scheduler) {
		t.Fatalf("wide footprint steady state allocs=%v got=%d", allocs, gotAgain)
	}
}

func TestDiagnosticParserCoreCanonicalKeyAndGroupSnapshotsAreAccounted(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	keySnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	groupSnapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	keyRegion := &diagnosticParserCoreS3Region{children: make([]core.SubtreeID, 1, 3)}
	groupRegion := &diagnosticParserCoreS3Region{children: make([]core.SubtreeID, 1, 4)}
	key := diagnosticParserCorePhaseHead{head: head, versionState: &diagnosticParserCoreVersionState{
		s3Region: keyRegion, relexSnapshot: keySnapshot,
	}}
	groupKey := diagnosticParserCorePhaseHead{head: head, versionState: &diagnosticParserCoreVersionState{
		s3Region: groupRegion, relexSnapshot: groupSnapshot,
	}}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		canonicalScratch: diagnosticParserCoreCanonicalScratch{
			keys:   []diagnosticParserCorePhaseHead{key},
			groups: map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup{groupKey: {}},
		},
	}
	base := diagnosticParserCoreSchedulerFootprintBytes(&diagnosticParserCoreGenericScheduler{compact: compact})
	got := diagnosticParserCoreSchedulerFootprintBytes(scheduler) - base
	minimum := uint64(2)*uint64(unsafe.Sizeof(diagnosticParserCoreVersionLexerSnapshot{})) +
		uint64(2)*uint64(unsafe.Sizeof(diagnosticParserCoreVersionState{})) +
		diagnosticParserCoreVersionS3RegionFootprintBytes(keyRegion) +
		diagnosticParserCoreVersionS3RegionFootprintBytes(groupRegion)
	if got < minimum {
		t.Fatalf("canonical key/group version state was not accounted: delta=%d minimum=%d", got, minimum)
	}
	if len(scheduler.footprintRefs) != 0 {
		t.Fatalf("footprint refs retained logical entries: len=%d", len(scheduler.footprintRefs))
	}
}

func TestDiagnosticParserCoreCanonicalGroupsReleaseSnapshotKeysOnNarrowPaths(t *testing.T) {
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	wide := make([]diagnosticParserCoreHeader, diagnosticParserCoreLinearCanonicalLimit+1)
	for index := range wide {
		wide[index] = diagnosticParserCoreHeader{
			head: head,
			versionState: &diagnosticParserCoreVersionState{
				relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{},
			},
		}
	}
	var scratch diagnosticParserCoreCanonicalScratch
	out, err := scratch.canonicalize(compact, wide)
	if err != nil || len(out) != len(wide) || len(scratch.groups) != len(wide) {
		t.Fatalf("mapped canonicalization output/groups=%d/%d err=%v, want %d/%d", len(out), len(scratch.groups), err, len(wide), len(wide))
	}
	if _, err := scratch.canonicalize(compact, wide[:1]); err != nil {
		t.Fatal(err)
	}
	if len(scratch.groups) != 0 {
		t.Fatalf("single-header canonicalization retained %d snapshot keys", len(scratch.groups))
	}
	if _, err := scratch.canonicalize(compact, wide); err != nil {
		t.Fatal(err)
	}
	if len(scratch.groups) != len(wide) {
		t.Fatalf("mapped canonicalization repopulated %d groups, want %d", len(scratch.groups), len(wide))
	}
	if _, err := scratch.canonicalizeRecovery(compact, wide[:2]); err != nil {
		t.Fatal(err)
	}
	if len(scratch.groups) != 0 {
		t.Fatalf("recovery canonicalization retained %d snapshot keys", len(scratch.groups))
	}
	if len(scratch.keys) != 0 {
		t.Fatalf("recovery canonicalization retained %d stale phase keys", len(scratch.keys))
	}
}
