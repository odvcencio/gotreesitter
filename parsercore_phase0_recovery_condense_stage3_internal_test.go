//go:build gts_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newStage3RecoveryCondenseScheduler(t *testing.T, regionEnds []uint32) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]diagnosticParserCoreHeader, 0, len(regionEnds))
	for index, endByte := range regionEnds {
		head, seedErr := compact.Seed(core.StateID(index+1), 0)
		if seedErr != nil {
			t.Fatal(seedErr)
		}
		child, childErr := compact.ErrorRegionLeaf(2, 0, endByte, false)
		if childErr != nil {
			t.Fatal(childErr)
		}
		header := diagnosticParserCoreHeader{head: head, creationSeq: uint64(index + 1)}
		header.openRecoveryRegion(&diagnosticParserCoreS3Region{
			state: core.StateID(index + 1), startByte: 0, endByte: endByte,
			children: []core.SubtreeID{child},
		})
		header.markRecoveryLineage()
		headers = append(headers, header)
	}
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 16)}
	for index := range lang.SymbolMetadata {
		lang.SymbolMetadata[index] = SymbolMetadata{Visible: true, Named: true}
	}
	return &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, recoveryIsolation: true,
		tokenSource: &dfaTokenSource{language: lang},
		options: DiagnosticParserCorePrefixOptions{
			allowCompactRecoveryLineageSelection: true,
			materializationSource:                []byte("abcdefgh"),
		},
	}
}

func runStage3RecoveryCondense(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) {
	t.Helper()
	if err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return scheduler.canonicalizeOwned(owner)
	}); err != nil {
		t.Fatal(err)
	}
}

func stage3RecoveryCondenseStates(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) []StateID {
	t.Helper()
	states := make([]StateID, 0, len(scheduler.headers))
	for index := range scheduler.headers {
		state, _, err := scheduler.compact.Boundary(scheduler.headers[index].head)
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, StateID(state))
	}
	return states
}

func TestStage3RecoveryCondenseOrdersBeforeSixVersionCap(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{2, 3, 4, 5, 6, 7, 1})
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{7, 1, 2, 3, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recovery condense states=%v, want C cost order and six-version cap %v", got, want)
	}
	if scheduler.work.RecoveryCondensePasses != 1 || scheduler.work.RecoveryVersionCapDrops != 1 {
		t.Fatalf("recovery condense telemetry=%+v, want one pass and one cap drop", scheduler.work)
	}
	for index := range scheduler.headers {
		if !scheduler.headers[index].isRecoveryLineage() || !scheduler.headers[index].isRecoveryCosted() || scheduler.headers[index].recoveryRegion() == nil {
			t.Fatalf("retained recovery header %d lost ownership: %+v", index, scheduler.headers[index])
		}
	}
	for index := len(scheduler.headers); index < cap(scheduler.headers); index++ {
		if scheduler.headers[:cap(scheduler.headers)][index].versionState != nil {
			t.Fatalf("dropped recovery header %d retained version state", index)
		}
	}
}

func TestStage3RecoveryCondenseKeepsSixDistinctVersions(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{6, 5, 4, 3, 2, 1})
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{6, 5, 4, 3, 2, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("six-version recovery order=%v, want %v", got, want)
	}
	if scheduler.work.RecoveryCondensePasses != 1 || scheduler.work.RecoveryVersionCapDrops != 0 {
		t.Fatalf("six-version telemetry=%+v, want one pass and no drop", scheduler.work)
	}
}

func TestStage3RecoveryCondensePairwiseDeletesClearlyWorseVersion(t *testing.T) {
	clean := diagnosticParserCoreRecoveryCondenseEntry{
		header: diagnosticParserCoreHeader{creationSeq: 1},
		status: core.RecoveryErrorStatus{Cost: 100, IsInError: false},
		key:    diagnosticParserCoreRecoveryCondenseKey{state: 1},
	}
	recovery := diagnosticParserCoreRecoveryCondenseEntry{
		header: diagnosticParserCoreHeader{creationSeq: 2},
		status: core.RecoveryErrorStatus{Cost: 2000, IsInError: true},
		key:    diagnosticParserCoreRecoveryCondenseKey{state: 2},
	}
	for _, test := range []struct {
		name    string
		entries []diagnosticParserCoreRecoveryCondenseEntry
		want    int
	}{
		{name: "take_left", entries: []diagnosticParserCoreRecoveryCondenseEntry{clean, recovery}, want: 0},
		{name: "take_right", entries: []diagnosticParserCoreRecoveryCondenseEntry{recovery, clean}, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			order := diagnosticParserCoreRecoveryCondensePairwise(test.entries, []int{0, 1})
			if len(order) != 1 || order[0] != test.want {
				t.Fatalf("pairwise order=%v, want only input %d", order, test.want)
			}
		})
	}
}

func TestStage3RecoveryCondenseEquivalentKeyUsesMergedNodeCountAndPrecedence(t *testing.T) {
	key := diagnosticParserCoreRecoveryCondenseKey{state: 1, byteOffset: 4, cost: 100}
	entries := []diagnosticParserCoreRecoveryCondenseEntry{
		{key: key, status: core.RecoveryErrorStatus{Cost: 100, NodeCount: 0, DynPrec: 1}},
		{key: key, status: core.RecoveryErrorStatus{Cost: 100, NodeCount: 20, DynPrec: 7}},
		{
			key:    diagnosticParserCoreRecoveryCondenseKey{state: 2, byteOffset: 4, cost: 200},
			status: core.RecoveryErrorStatus{Cost: 200},
		},
	}
	diagnosticParserCoreMergeEquivalentRecoveryStatus(&entries[0], entries[1])
	if entries[0].status.NodeCount != 20 || entries[0].status.DynPrec != 7 {
		t.Fatalf("merged equivalent status=%+v, want maximum node count and precedence", entries[0].status)
	}
	if got := diagnosticParserCoreRecoveryCondensePairwise(entries, []int{0, 2}); !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("merged-node-count comparison order=%v, want lower-cost merged key to take competitor", got)
	}
}

func TestStage3RecoveryCondenseKeyIgnoresDynamicPrecedence(t *testing.T) {
	key := diagnosticParserCoreRecoveryCondenseKey{
		state: 3, byteOffset: 4, cost: 500, checkpoint: 6,
		shifted: true, paused: true,
	}
	left := diagnosticParserCoreRecoveryCondenseEntry{
		key: key, status: core.RecoveryErrorStatus{Cost: 500, DynPrec: 1},
	}
	right := diagnosticParserCoreRecoveryCondenseEntry{
		key: key, status: core.RecoveryErrorStatus{Cost: 500, DynPrec: 9},
	}
	if left.key != right.key {
		t.Fatal("dynamic precedence split one C recovery merge key")
	}
	diagnosticParserCoreMergeEquivalentRecoveryStatus(&left, right)
	if left.status.DynPrec != 9 {
		t.Fatalf("merged dynamic precedence=%d, want 9", left.status.DynPrec)
	}
}

func TestStage3RecoveryCondenseDifferentScoresShareOneVersionKey(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 2}},
			{state: 2, symbol: 9}: {{
				Type: core.ActionReduce, Symbol: 10, ChildCount: 1,
				DynamicPrecedence: 7,
			}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 10}: 3},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := compact.Shift(seed, 8, 0, core.Token{Symbol: 8}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := compact.ReduceOutputs(shifted, 9, 0, core.ForkOrder{})
	if err != nil || len(outputs) != 1 {
		t.Fatalf("scored reduction outputs=%+v error=%v", outputs, err)
	}

	makeHeader := func(head core.Head, sequence uint64, endByte uint32) diagnosticParserCoreHeader {
		child, childErr := compact.ErrorRegionLeaf(2, 0, endByte, false)
		if childErr != nil {
			t.Fatal(childErr)
		}
		header := diagnosticParserCoreHeader{head: head, creationSeq: sequence}
		header.openRecoveryRegion(&diagnosticParserCoreS3Region{
			state: 1, startByte: 0, endByte: endByte, children: []core.SubtreeID{child},
		})
		header.markRecoveryLineage()
		return header
	}
	headers := []diagnosticParserCoreHeader{makeHeader(outputs[0].Head, 1, 1)}
	for index, endByte := range []uint32{1, 2, 3, 4, 5, 6} {
		head, seedErr := compact.Seed(core.StateID(index+4), 0)
		if seedErr != nil {
			t.Fatal(seedErr)
		}
		headers = append(headers, makeHeader(head, uint64(index+2), endByte))
	}
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 16)}
	for index := range lang.SymbolMetadata {
		lang.SymbolMetadata[index] = SymbolMetadata{Visible: true, Named: true}
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, recoveryIsolation: true,
		tokenSource: &dfaTokenSource{language: lang},
		options: DiagnosticParserCorePrefixOptions{
			allowCompactRecoveryLineageSelection: true,
			materializationSource:                []byte("abcdef"),
		},
	}
	runStage3RecoveryCondense(t, scheduler)
	if len(scheduler.headers) != 7 || scheduler.work.RecoveryVersionCapDrops != 0 {
		t.Fatalf("score-equivalent recovery keys left %d histories with %d cap drops, want 7 and 0",
			len(scheduler.headers), scheduler.work.RecoveryVersionCapDrops)
	}
}

func TestStage3RecoveryCondenseUsesLiveNodeCountForTakeDecision(t *testing.T) {
	for _, test := range []struct {
		name string
		ends []uint32
		want StateID
	}{
		{name: "take_left", ends: []uint32{1, 1001}, want: 1},
		{name: "take_right", ends: []uint32{1001, 1}, want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheduler := newStage3RecoveryCondenseScheduler(t, test.ends)
			runStage3RecoveryCondense(t, scheduler)
			if got := stage3RecoveryCondenseStates(t, scheduler); !reflect.DeepEqual(got, []StateID{test.want}) {
				t.Fatalf("node-count recovery decision=%v, want [%d]", got, test.want)
			}
			if cap(scheduler.headers) > len(scheduler.headers) &&
				scheduler.headers[:cap(scheduler.headers)][len(scheduler.headers)].versionState != nil {
				t.Fatal("pairwise recovery drop retained loser ownership")
			}
		})
	}
}

func TestStage3OpenRegionVisibleNodeCountRequiresOneAbsorbedChild(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1})
	src, err := newDiagnosticParserCoreRecoveryCostSource(
		scheduler.compact, scheduler.options.materializationSource,
	)
	if err != nil {
		t.Fatal(err)
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(scheduler.tokenSource.language)
	if got, err := diagnosticParserCoreOpenRegionVisibleNodeCount(
		symbols, src, &diagnosticParserCoreS3Region{},
	); err != nil || got != 0 {
		t.Fatalf("empty open-region count=%d error=%v, want 0", got, err)
	}
	region := scheduler.headers[0].recoveryRegion()
	if got, err := diagnosticParserCoreOpenRegionVisibleNodeCount(symbols, src, region); err != nil || got != 2 {
		t.Fatalf("non-empty open-region count=%d error=%v, want hidden wrapper plus child", got, err)
	}
}

func TestStage3RecoveryCondensePreservesAbsorbBeforeItsMissingSibling(t *testing.T) {
	absorb := diagnosticParserCoreRecoveryCondenseEntry{
		header: diagnosticParserCoreHeader{creationSeq: 1},
		status: core.RecoveryErrorStatus{Cost: 2000, IsInError: true},
		key: diagnosticParserCoreRecoveryCondenseKey{
			state: 0, recoveryGroup: 9,
		},
	}
	missing := diagnosticParserCoreRecoveryCondenseEntry{
		header: diagnosticParserCoreHeader{creationSeq: 2},
		status: core.RecoveryErrorStatus{Cost: 100, IsInError: false},
		key: diagnosticParserCoreRecoveryCondenseKey{
			state: 2, missingGroup: 9,
		},
	}
	for _, test := range []struct {
		name    string
		entries []diagnosticParserCoreRecoveryCondenseEntry
		want    []int
	}{
		{name: "already_ordered", entries: []diagnosticParserCoreRecoveryCondenseEntry{absorb, missing}, want: []int{0, 1}},
		{name: "restore_order", entries: []diagnosticParserCoreRecoveryCondenseEntry{missing, absorb}, want: []int{1, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			order := diagnosticParserCoreRecoveryCondensePairwise(test.entries, []int{0, 1})
			if !reflect.DeepEqual(order, test.want) {
				t.Fatalf("S5 group order=%v, want %v", order, test.want)
			}
		})
	}
}

func TestStage3RecoveryCondenseSkipsCostCompetitionWithinRecoveryGroup(t *testing.T) {
	entries := []diagnosticParserCoreRecoveryCondenseEntry{
		{
			header: diagnosticParserCoreHeader{creationSeq: 1},
			status: core.RecoveryErrorStatus{Cost: 100, IsInError: true},
			key:    diagnosticParserCoreRecoveryCondenseKey{state: 1, recoveryGroup: 7},
		},
		{
			header: diagnosticParserCoreHeader{creationSeq: 2},
			status: core.RecoveryErrorStatus{Cost: 4000, IsInError: true},
			key:    diagnosticParserCoreRecoveryCondenseKey{state: 2, recoveryGroup: 7},
		},
	}
	if got := diagnosticParserCoreRecoveryCondensePairwise(entries, []int{0, 1}); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("same-group recovery order=%v, want both histories", got)
	}
}

func TestStage3RecoveryCondenseClampsBaselineAfterGraphPop(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1, 2})
	for index := range scheduler.headers {
		header := &scheduler.headers[index]
		header.closeRecoveryRegion()
		header.publishRecoveryCondenseState(0, 0, 0, true)
	}
	scheduler.headers[0].publishRecoveryCondenseState(0, 0, 5, true)
	runStage3RecoveryCondense(t, scheduler)

	for index := range scheduler.headers {
		state, _, err := scheduler.compact.Boundary(scheduler.headers[index].head)
		if err != nil {
			t.Fatal(err)
		}
		if state != 1 {
			continue
		}
		baseline, set := scheduler.headers[index].recoveryNodeBaseline()
		if !set || baseline != 0 {
			t.Fatalf("clamped baseline=%d/%t, want 0/true", baseline, set)
		}
		return
	}
	t.Fatal("clamped recovery header was removed")
}

func TestStage3RecoveryMetadataSurvivesLexerStateUpdates(t *testing.T) {
	absorb := diagnosticParserCoreHeader{}
	absorb.openRecoveryRegion(&diagnosticParserCoreS3Region{state: 2, startByte: 1, endByte: 2})
	absorb.markRecoveryLineage()
	absorb.publishRecoveryCondenseState(9, 0, 7, true)
	snapshot := &diagnosticParserCoreVersionLexerSnapshot{}
	absorb.installVersionLexerSnapshot(snapshot)
	absorb.setRecoveryRegion(&diagnosticParserCoreS3Region{state: 2, startByte: 1, endByte: 3})
	if absorb.versionLexerSnapshot() != snapshot || absorb.recoveryGroupIdentity() != 9 {
		t.Fatalf("absorb lexer update lost recovery group: %+v", absorb.versionState)
	}
	if baseline, set := absorb.recoveryNodeBaseline(); !set || baseline != 7 {
		t.Fatalf("absorb lexer update baseline=%d/%t, want 7/true", baseline, set)
	}
	absorb.closeRecoveryRegion()
	if absorb.recoveryGroupIdentity() != 0 {
		t.Fatalf("closed absorb retained recovery group %d", absorb.recoveryGroupIdentity())
	}
	if baseline, set := absorb.recoveryNodeBaseline(); !set || baseline != 7 {
		t.Fatalf("closed absorb baseline=%d/%t, want 7/true", baseline, set)
	}

	missing := diagnosticParserCoreHeader{}
	missing.markRecoveryLineage()
	missing.publishRecoveryCondenseState(0, 9, 5, true)
	missing.installVersionLexerSnapshot(snapshot)
	missing.clearVersionLexerSnapshot()
	if missing.recoveryMissingGroupIdentity() != 9 {
		t.Fatalf("missing lexer update lost group %d", missing.recoveryMissingGroupIdentity())
	}
	if baseline, set := missing.recoveryNodeBaseline(); !set || baseline != 5 {
		t.Fatalf("missing lexer update baseline=%d/%t, want 5/true", baseline, set)
	}
}

func TestStage3RecoveryCondenseRetainsDuplicateHistoryOfKeptKey(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1, 2, 3, 4, 5, 6, 1})
	firstChild := scheduler.headers[0].recoveryRegion().children[0]
	duplicateChild := scheduler.headers[6].recoveryRegion().children[0]
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{1, 7, 2, 3, 4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate-key recovery order=%v, want retained C merge histories %v", got, want)
	}
	if scheduler.work.RecoveryVersionCapDrops != 0 || len(scheduler.headers) != 7 {
		t.Fatalf("duplicate-key recovery frontier=%d telemetry=%+v, want seven histories across six keys", len(scheduler.headers), scheduler.work)
	}
	if firstChild == duplicateChild || scheduler.headers[0].recoveryRegion().children[0] != firstChild ||
		scheduler.headers[1].recoveryRegion().children[0] != duplicateChild {
		t.Fatalf("duplicate C key lost one recovery history: first=%d duplicate=%d headers=%+v", firstChild, duplicateChild, scheduler.headers)
	}
}

func TestStage3RecoveryCondenseDropsOnlyExcessKeyAfterDuplicate(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1, 2, 3, 4, 5, 6, 1, 7})
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{1, 7, 2, 3, 4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate-before-excess order=%v, want %v", got, want)
	}
	if scheduler.work.RecoveryVersionCapDrops != 1 || len(scheduler.headers) != 7 {
		t.Fatalf("duplicate-before-excess frontier=%d telemetry=%+v, want one excess-key drop", len(scheduler.headers), scheduler.work)
	}
}

func TestStage3RecoveryCondenseUsesScannerCheckpointInsteadOfSnapshotPointer(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1, 1, 2, 3, 4, 5, 6, 7})
	for index := 0; index < 2; index++ {
		header := &scheduler.headers[index]
		baseline, baselineSet := header.recoveryNodeBaseline()
		header.publishVersionState(
			header.recoveryRegion(), &diagnosticParserCoreVersionLexerSnapshot{}, 0,
			header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(),
			baseline, baselineSet,
		)
	}
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{1, 2, 3, 4, 5, 6, 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("equal scanner checkpoint with cloned snapshots=%v, want %v", got, want)
	}
	if scheduler.work.RecoveryVersionCapDrops != 1 || len(scheduler.headers) != 7 {
		t.Fatalf("cloned-snapshot cap frontier=%d telemetry=%+v, want seven histories and one drop", len(scheduler.headers), scheduler.work)
	}
}

func TestStage3RecoveryCondenseAppendsAcceptedVersionsAfterActivePool(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{2, 1, 3})
	scheduler.headers[0].accepted = true
	runStage3RecoveryCondense(t, scheduler)

	if got, want := stage3RecoveryCondenseStates(t, scheduler), []StateID{2, 3, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("active and accepted recovery order=%v, want C pool order %v", got, want)
	}
	if scheduler.headers[0].accepted || scheduler.headers[1].accepted || !scheduler.headers[2].accepted {
		t.Fatalf("accepted recovery version did not remain outside the active pool: %+v", scheduler.headers)
	}
}

func TestStage3RecoveryCondenseErrorRollsBackConflictAndOwnership(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{MaxDerivations: 8})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	header := diagnosticParserCoreHeader{head: head, creationSeq: 4}
	header.openRecoveryRegion(&diagnosticParserCoreS3Region{
		state: 1, startByte: 0, endByte: 1,
		children: []core.SubtreeID{core.SubtreeID(^uint32(0))},
	})
	header.markRecoveryLineage()
	headers := []diagnosticParserCoreHeader{header}
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 8)}
	for index := range lang.SymbolMetadata {
		lang.SymbolMetadata[index] = SymbolMetadata{Visible: true, Named: true}
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: append([]diagnosticParserCoreHeader(nil), headers...),
		token:       Token{Symbol: 9, StartByte: 0, EndByte: 1},
		branchOrder: 7, nextSeq: 10, recoveryIsolation: true,
		tokenSource: &dfaTokenSource{language: lang},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches:                        100,
			allowCompactRecoveryLineageSelection: true,
			materializationSource:                []byte("x"),
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	statsBefore, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, headers[0], 9)
	if err := scheduler.applyGenericConflict(before, cell); err == nil {
		t.Fatal("invalid open-region child unexpectedly passed recovery condensation")
	}
	statsAfter, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter != statsBefore || !reflect.DeepEqual(scheduler.headers, headers) ||
		scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.dispatches != 0 ||
		scheduler.work != (DiagnosticParserCoreGenericWork{}) ||
		!reflect.DeepEqual(scheduler.receipt, &DiagnosticParserCoreGenericScheduler{}) {
		t.Fatalf("recovery condense rollback leaked: before=%+v after=%+v scheduler=%+v", statsBefore, statsAfter, scheduler)
	}
	if len(scheduler.recoveryCondenseScratch) != 0 {
		t.Fatalf("recovery condense scratch length=%d after rollback", len(scheduler.recoveryCondenseScratch))
	}
	for index := range scheduler.recoveryCondenseScratch[:cap(scheduler.recoveryCondenseScratch)] {
		if scheduler.recoveryCondenseScratch[:cap(scheduler.recoveryCondenseScratch)][index].header.versionState != nil {
			t.Fatalf("recovery condense scratch entry %d retained version state", index)
		}
	}
}

func TestStage3RecoveryCondenseScratchIsCountedAndClearedOnReset(t *testing.T) {
	scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{1})
	entry := diagnosticParserCoreRecoveryCondenseEntry{header: scheduler.headers[0]}
	scheduler.recoveryCondenseScratch = append(
		make([]diagnosticParserCoreRecoveryCondenseEntry, 0, 7), entry,
	)
	withScratch := diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	scheduler.recoveryCondenseScratch = nil
	withoutScratch := diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	wantDelta := uint64(7 * unsafe.Sizeof(diagnosticParserCoreRecoveryCondenseEntry{}))
	if withScratch-withoutScratch != wantDelta {
		t.Fatalf("recovery scratch footprint delta=%d, want %d", withScratch-withoutScratch, wantDelta)
	}
	scheduler.recoveryCondenseOrderScratch = append(make([]int, 0, 5), 3)
	withOrder := diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	scheduler.recoveryCondenseOrderScratch = nil
	withoutOrder := diagnosticParserCoreSchedulerFootprintBytes(scheduler)
	wantOrderDelta := uint64(5 * unsafe.Sizeof(int(0)))
	if withOrder-withoutOrder != wantOrderDelta {
		t.Fatalf("recovery order scratch footprint delta=%d, want %d", withOrder-withoutOrder, wantOrderDelta)
	}

	scheduler.recoveryCondenseScratch = append(
		make([]diagnosticParserCoreRecoveryCondenseEntry, 0, 7), entry,
	)
	scheduler.recoveryCondenseOrderScratch = append(make([]int, 0, 5), 3)
	if err := resetDiagnosticParserCoreGenericScheduler(scheduler); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.recoveryCondenseScratch) != 0 || cap(scheduler.recoveryCondenseScratch) != 7 {
		t.Fatalf("reset recovery scratch len/cap=%d/%d, want 0/7", len(scheduler.recoveryCondenseScratch), cap(scheduler.recoveryCondenseScratch))
	}
	for index := range scheduler.recoveryCondenseScratch[:cap(scheduler.recoveryCondenseScratch)] {
		if scheduler.recoveryCondenseScratch[:cap(scheduler.recoveryCondenseScratch)][index].header.versionState != nil {
			t.Fatalf("reset recovery scratch entry %d retained version state", index)
		}
	}
	if len(scheduler.recoveryCondenseOrderScratch) != 0 || cap(scheduler.recoveryCondenseOrderScratch) != 5 {
		t.Fatalf("reset recovery order scratch len/cap=%d/%d, want 0/5",
			len(scheduler.recoveryCondenseOrderScratch), cap(scheduler.recoveryCondenseOrderScratch))
	}
	for index, value := range scheduler.recoveryCondenseOrderScratch[:cap(scheduler.recoveryCondenseOrderScratch)] {
		if value != 0 {
			t.Fatalf("reset recovery order scratch entry %d=%d, want zero", index, value)
		}
	}
}
