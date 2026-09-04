package parsercorephase0

import (
	"reflect"
	"testing"
)

func TestResetRetainsConfigurationAndArenaCapacity(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 1, symbol: 9}:  {{Type: ActionShift, State: 2}},
			{state: 2, symbol: 10}: {{Type: ActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 7}},
		},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 4}: 3},
		fields:  map[uint16][]FieldMapEntry{7: {{FieldID: 2, ChildIndex: 0}}},
		aliases: map[productionKey][]Symbol{{productionID: 7, childCount: 1}: {11}},
	}
	limits := Limits{
		MaxNodes: 32, MaxLinks: 32, MaxSubtrees: 32, MaxChildren: 32,
		MaxMetadata: 32, MaxLinksPerBoundary: 4, MaxPopPaths: 8, MaxDerivations: 8,
	}
	compact, err := New(tables, limits)
	if err != nil {
		t.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := compact.Shift(seed, 9, 0, Token{Symbol: 9, EndByte: 1}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	frontier, err := compact.ReduceOutputs(shifted, 10, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("reduction frontier=%+v err=%v", frontier, err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	if err := compact.SetPhaseCheckpoint(mustInternCheckpoint(t, compact, []byte{1, 2, 3})); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.Seed(4, 2); err != nil {
		t.Fatal(err)
	}
	if compact.Work() == (Work{}) || compact.boundaries.count == 0 {
		t.Fatalf("reset fixture was not populated: work=%+v boundaries=%d", compact.Work(), compact.boundaries.count)
	}

	wantTables := compact.tables
	wantLimits := compact.limits
	wantDiagnostics := compact.diagnostics
	wantCaps := [...]int{
		cap(compact.nodes), cap(compact.nodeCheckpoints), cap(compact.links), cap(compact.subtrees),
		cap(compact.externalProvenance),
		cap(compact.missingLeafProvenance),
		cap(compact.lexerSkippedPrefixes),
		cap(compact.children), cap(compact.fields), cap(compact.aliases),
		cap(compact.boundaries.slots), cap(compact.boundaryJournal), cap(compact.transactions),
		cap(compact.reductionScratch.boundaries), cap(compact.reductionScratch.batchParents),
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}

	if compact.tables != wantTables || compact.limits != wantLimits || compact.diagnostics != wantDiagnostics {
		t.Fatalf("reset changed retained configuration: tables=%v limits=%+v diagnostics=%+v", compact.tables == wantTables, compact.limits, compact.diagnostics)
	}
	gotCaps := [...]int{
		cap(compact.nodes), cap(compact.nodeCheckpoints), cap(compact.links), cap(compact.subtrees),
		cap(compact.externalProvenance),
		cap(compact.missingLeafProvenance),
		cap(compact.lexerSkippedPrefixes),
		cap(compact.children), cap(compact.fields), cap(compact.aliases),
		cap(compact.boundaries.slots), cap(compact.boundaryJournal), cap(compact.transactions),
		cap(compact.reductionScratch.boundaries), cap(compact.reductionScratch.batchParents),
	}
	if gotCaps != wantCaps {
		t.Fatalf("reset changed retained capacities: got=%v want=%v", gotCaps, wantCaps)
	}
	if len(compact.nodes) != 0 || len(compact.nodeCheckpoints) != 0 || len(compact.links) != 0 || len(compact.subtrees) != 0 || len(compact.externalProvenance) != 0 || len(compact.missingLeafProvenance) != 0 || len(compact.lexerSkippedPrefixes) != 0 || len(compact.children) != 0 || len(compact.fields) != 0 || len(compact.aliases) != 0 || compact.boundaries.count != 0 || len(compact.boundaryJournal) != 0 || len(compact.transactions) != 0 {
		t.Fatalf("reset retained logical state: nodes=%d node_checkpoints=%d links=%d subtrees=%d external_provenance=%d lexer_skipped_prefixes=%d children=%d fields=%d aliases=%d boundaries=%d journal=%d transactions=%d", len(compact.nodes), len(compact.nodeCheckpoints), len(compact.links), len(compact.subtrees), len(compact.externalProvenance), len(compact.lexerSkippedPrefixes), len(compact.children), len(compact.fields), len(compact.aliases), compact.boundaries.count, len(compact.boundaryJournal), len(compact.transactions))
	}
	if compact.frontier != 1 || compact.checkpoint != 0 || compact.nextTransaction != 0 || compact.Work() != (Work{}) {
		t.Fatalf("reset scalar drift: frontier=%d checkpoint=%x next_transaction=%d work=%+v", compact.frontier, compact.checkpoint, compact.nextTransaction, compact.Work())
	}
	if compact.reductionScratch.spilled || len(compact.reductionScratch.boundaries) != 0 || len(compact.reductionScratch.boundaryByKey) != 0 || len(compact.reductionScratch.batchParents) != 0 {
		t.Fatalf("reset retained logical reduction scratch: %+v", compact.reductionScratch)
	}
	if _, _, err := compact.Boundary(seed); err == nil {
		t.Fatal("pre-reset head remained valid")
	}
	newSeed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if newSeed.Node != 1 {
		t.Fatalf("new seed node=%d, want 1", newSeed.Node)
	}
}

func TestResetRejectsNilAndActiveTransactionWithoutMutation(t *testing.T) {
	var nilCore *Core
	if err := nilCore.Reset(); err == nil {
		t.Fatal("nil reset unexpectedly succeeded")
	}

	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.ApplyAtomic(func() error {
		beforeNodes := append([]nodeRecord(nil), compact.nodes...)
		beforeNodeCheckpoints := append([]CheckpointID(nil), compact.nodeCheckpoints...)
		beforeLinks := append([]linkRecord(nil), compact.links...)
		beforeSubtrees := append([]subtreeRecord(nil), compact.subtrees...)
		beforeExternalProvenance := append([]externalPayloadProvenance(nil), compact.externalProvenance...)
		beforeMissingLeafProvenance := append([]missingLeafProvenance(nil), compact.missingLeafProvenance...)
		beforeLexerSkippedPrefixes := append([]lexerSkippedPrefixProvenance(nil), compact.lexerSkippedPrefixes...)
		beforeChildren := append([]SubtreeID(nil), compact.children...)
		beforeFields := append([]FieldMapEntry(nil), compact.fields...)
		beforeAliases := append([]Symbol(nil), compact.aliases...)
		beforeBoundaries := cloneBoundaryMap(compact.boundaries)
		beforeBoundaryIndex := compact.boundaries.snapshot()
		beforeJournal := append([]boundaryMutation(nil), compact.boundaryJournal...)
		beforeTransactions := append([]uint64(nil), compact.transactions...)
		beforeFrontier, beforeCheckpoint := compact.frontier, compact.checkpoint
		beforeNext, beforeWork := compact.nextTransaction, compact.work

		if err := compact.Reset(); err == nil {
			t.Fatal("reset during active transaction unexpectedly succeeded")
		}
		if !reflect.DeepEqual(compact.nodes, beforeNodes) || !reflect.DeepEqual(compact.nodeCheckpoints, beforeNodeCheckpoints) || !reflect.DeepEqual(compact.links, beforeLinks) || !reflect.DeepEqual(compact.subtrees, beforeSubtrees) || !reflect.DeepEqual(compact.externalProvenance, beforeExternalProvenance) || !reflect.DeepEqual(compact.missingLeafProvenance, beforeMissingLeafProvenance) || !reflect.DeepEqual(compact.lexerSkippedPrefixes, beforeLexerSkippedPrefixes) || !reflect.DeepEqual(compact.children, beforeChildren) || !reflect.DeepEqual(compact.fields, beforeFields) || !reflect.DeepEqual(compact.aliases, beforeAliases) || !reflect.DeepEqual(compact.boundaries.logicalMap(), beforeBoundaries) || !reflect.DeepEqual(compact.boundaries.snapshot(), beforeBoundaryIndex) || !reflect.DeepEqual(compact.boundaryJournal, beforeJournal) || !reflect.DeepEqual(compact.transactions, beforeTransactions) || compact.frontier != beforeFrontier || compact.checkpoint != beforeCheckpoint || compact.nextTransaction != beforeNext || compact.work != beforeWork {
			t.Fatal("rejected active reset mutated compact state")
		}
		if state, offset, err := compact.Boundary(seed); err != nil || state != 1 || offset != 0 {
			t.Fatalf("rejected active reset invalidated seed: state=%d offset=%d err=%v", state, offset, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
