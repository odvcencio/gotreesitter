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
	compact.SetPhaseCheckpoint([32]byte{1, 2, 3})
	if compact.Work() == (Work{}) || len(compact.boundaries) == 0 {
		t.Fatalf("reset fixture was not populated: work=%+v boundaries=%d", compact.Work(), len(compact.boundaries))
	}

	wantTables := compact.tables
	wantLimits := compact.limits
	wantDiagnostics := compact.diagnostics
	wantCaps := [...]int{
		cap(compact.nodes), cap(compact.links), cap(compact.subtrees),
		cap(compact.children), cap(compact.fields), cap(compact.aliases),
		cap(compact.boundaryJournal), cap(compact.transactions),
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}

	if compact.tables != wantTables || compact.limits != wantLimits || compact.diagnostics != wantDiagnostics {
		t.Fatalf("reset changed retained configuration: tables=%v limits=%+v diagnostics=%+v", compact.tables == wantTables, compact.limits, compact.diagnostics)
	}
	gotCaps := [...]int{
		cap(compact.nodes), cap(compact.links), cap(compact.subtrees),
		cap(compact.children), cap(compact.fields), cap(compact.aliases),
		cap(compact.boundaryJournal), cap(compact.transactions),
	}
	if gotCaps != wantCaps {
		t.Fatalf("reset changed retained capacities: got=%v want=%v", gotCaps, wantCaps)
	}
	if len(compact.nodes) != 0 || len(compact.links) != 0 || len(compact.subtrees) != 0 || len(compact.children) != 0 || len(compact.fields) != 0 || len(compact.aliases) != 0 || len(compact.boundaries) != 0 || len(compact.boundaryJournal) != 0 || len(compact.transactions) != 0 {
		t.Fatalf("reset retained logical state: nodes=%d links=%d subtrees=%d children=%d fields=%d aliases=%d boundaries=%d journal=%d transactions=%d", len(compact.nodes), len(compact.links), len(compact.subtrees), len(compact.children), len(compact.fields), len(compact.aliases), len(compact.boundaries), len(compact.boundaryJournal), len(compact.transactions))
	}
	if compact.frontier != 1 || compact.checkpoint != ([32]byte{}) || compact.nextTransaction != 0 || compact.Work() != (Work{}) {
		t.Fatalf("reset scalar drift: frontier=%d checkpoint=%x next_transaction=%d work=%+v", compact.frontier, compact.checkpoint, compact.nextTransaction, compact.Work())
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
		beforeLinks := append([]linkRecord(nil), compact.links...)
		beforeSubtrees := append([]subtreeRecord(nil), compact.subtrees...)
		beforeChildren := append([]SubtreeID(nil), compact.children...)
		beforeFields := append([]FieldMapEntry(nil), compact.fields...)
		beforeAliases := append([]Symbol(nil), compact.aliases...)
		beforeBoundaries := make(map[boundaryKey]NodeID, len(compact.boundaries))
		for key, value := range compact.boundaries {
			beforeBoundaries[key] = value
		}
		beforeJournal := append([]boundaryMutation(nil), compact.boundaryJournal...)
		beforeTransactions := append([]uint64(nil), compact.transactions...)
		beforeFrontier, beforeCheckpoint := compact.frontier, compact.checkpoint
		beforeNext, beforeWork := compact.nextTransaction, compact.work

		if err := compact.Reset(); err == nil {
			t.Fatal("reset during active transaction unexpectedly succeeded")
		}
		if !reflect.DeepEqual(compact.nodes, beforeNodes) || !reflect.DeepEqual(compact.links, beforeLinks) || !reflect.DeepEqual(compact.subtrees, beforeSubtrees) || !reflect.DeepEqual(compact.children, beforeChildren) || !reflect.DeepEqual(compact.fields, beforeFields) || !reflect.DeepEqual(compact.aliases, beforeAliases) || !reflect.DeepEqual(compact.boundaries, beforeBoundaries) || !reflect.DeepEqual(compact.boundaryJournal, beforeJournal) || !reflect.DeepEqual(compact.transactions, beforeTransactions) || compact.frontier != beforeFrontier || compact.checkpoint != beforeCheckpoint || compact.nextTransaction != beforeNext || compact.work != beforeWork {
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
