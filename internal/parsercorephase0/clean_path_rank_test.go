package parsercorephase0

import (
	"reflect"
	"testing"
)

type cleanPathRankBranch struct {
	predecessor StateID
	prefixScore []int64
	windowScore int64
	gotoState   StateID
	external    bool
	exact       bool
}

func newCleanPathRankFixture(tb testing.TB, branches []cleanPathRankBranch) (*Core, Head) {
	tb.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 50, symbol: 9}: {{
				Type: ActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 6,
			}},
		},
		gotos: make(map[tableCell]StateID, len(branches)),
	}
	for _, branch := range branches {
		tables.gotos[tableCell{state: branch.predecessor, symbol: 2}] = branch.gotoState
	}
	compact, err := New(tables, Limits{MaxDerivations: 16, MaxPopPaths: 16})
	if err != nil {
		tb.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false

	var head Head
	for branchIndex, branch := range branches {
		baseState := StateID(100 + branchIndex)
		branchHead, err := compact.Seed(baseState, 0)
		if err != nil {
			tb.Fatal(err)
		}
		for prefixIndex, score := range branch.prefixScore {
			payload, err := compact.appendSubtree(subtreeRecord{
				symbol:   Symbol(100 + prefixIndex),
				terminal: true,
			}, nil, nil, nil)
			if err != nil {
				tb.Fatal(err)
			}
			nextState := StateID(200 + branchIndex*16 + prefixIndex)
			if prefixIndex == len(branch.prefixScore)-1 {
				nextState = branch.predecessor
			}
			branchHead, err = compact.appendPrivate(nextState, 0, linkInput{
				prev: branchHead.Node, payload: payload, scoreDelta: score,
			})
			if err != nil {
				tb.Fatal(err)
			}
		}
		if len(branch.prefixScore) == 0 {
			branchHead, err = compact.Seed(branch.predecessor, 0)
			if err != nil {
				tb.Fatal(err)
			}
		}
		if branch.external && branch.exact {
			start, err := compact.InternCheckpoint([]byte{1, 2})
			if err != nil {
				tb.Fatal(err)
			}
			end, err := compact.InternCheckpoint([]byte{3, 4})
			if err != nil {
				tb.Fatal(err)
			}
			if err := compact.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
				tb.Fatal(err)
			}
		}
		childRecord := subtreeRecord{
			symbol: 10, startByte: 0, endByte: 1,
			terminal: true, external: branch.external,
		}
		var child SubtreeID
		if branch.exact {
			child, err = compact.appendAuthenticatedTerminal(childRecord)
		} else {
			child, err = compact.appendSubtree(childRecord, nil, nil, nil)
		}
		if err != nil {
			tb.Fatal(err)
		}
		head, err = compact.condense(compact.boundaryKey(50, 1), linkInput{
			prev: branchHead.Node, payload: child, scoreDelta: branch.windowScore,
		})
		if err != nil {
			tb.Fatal(err)
		}
	}
	return compact, head
}

func cleanPathRankOutputStates(tb testing.TB, compact *Core, outputs []ReductionOutput) map[StateID]CleanPathRankSelection {
	tb.Helper()
	got := make(map[StateID]CleanPathRankSelection, len(outputs))
	for _, output := range outputs {
		node, err := compact.node(output.Head.Node)
		if err != nil {
			tb.Fatal(err)
		}
		got[node.state] = output.CleanPathRank
	}
	return got
}

func newAmbiguousPrefixRankFixture(tb testing.TB) (*Core, Head) {
	tb.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 50, symbol: 9}: {{
				Type: ActionReduce, Symbol: 2, ChildCount: 1,
			}},
		},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 2}: 60,
			{state: 2, symbol: 2}: 61,
		},
	}
	compact, err := New(tables, Limits{MaxDerivations: 16, MaxPopPaths: 16})
	if err != nil {
		tb.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	leftSeed, _ := compact.Seed(10, 0)
	rightSeed, _ := compact.Seed(11, 0)
	other, _ := compact.Seed(2, 0)
	leftPayload, _ := compact.appendSubtree(
		subtreeRecord{symbol: 20, terminal: true}, nil, nil, nil,
	)
	rightPayload, _ := compact.appendSubtree(
		subtreeRecord{symbol: 21, terminal: true}, nil, nil, nil,
	)
	prefix, err := compact.condense(compact.boundaryKey(1, 0), linkInput{
		prev: leftSeed.Node, payload: leftPayload, scoreDelta: 5,
	})
	if err != nil {
		tb.Fatal(err)
	}
	prefix, err = compact.condense(compact.boundaryKey(1, 0), linkInput{
		prev: rightSeed.Node, payload: rightPayload, scoreDelta: 5,
	})
	if err != nil {
		tb.Fatal(err)
	}
	firstChild, _ := compact.appendSubtree(
		subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil,
	)
	secondChild, _ := compact.appendSubtree(
		subtreeRecord{symbol: 11, endByte: 1, terminal: true}, nil, nil, nil,
	)
	head, err := compact.condense(compact.boundaryKey(50, 1), linkInput{
		prev: prefix.Node, payload: firstChild,
	})
	if err != nil {
		tb.Fatal(err)
	}
	head, err = compact.condense(compact.boundaryKey(50, 1), linkInput{
		prev: other.Node, payload: secondChild, scoreDelta: 4,
	})
	if err != nil {
		tb.Fatal(err)
	}
	return compact, head
}

func TestCleanPathRankUsesScoreBeforeDepth(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60},
		{predecessor: 2, prefixScore: []int64{2, 2}, gotoState: 61},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	got := cleanPathRankOutputStates(t, compact, outputs)
	want := map[StateID]CleanPathRankSelection{
		60: CleanPathRankSelected,
		61: CleanPathRankUnselected,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked outputs=%v, want %v", got, want)
	}
}

func TestCleanPathRankUsesDepthAfterEqualScore(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{3}, gotoState: 60},
		{predecessor: 2, prefixScore: []int64{1, 2}, gotoState: 61},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	got := cleanPathRankOutputStates(t, compact, outputs)
	want := map[StateID]CleanPathRankSelection{
		60: CleanPathRankUnselected,
		61: CleanPathRankSelected,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked outputs=%v, want %v", got, want)
	}
}

func TestCleanPathRankExactCrossPathTieIsUnknown(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{3}, gotoState: 60},
		{predecessor: 2, prefixScore: []int64{3}, gotoState: 61},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	got := cleanPathRankOutputStates(t, compact, outputs)
	want := map[StateID]CleanPathRankSelection{
		60: CleanPathRankUnknown,
		61: CleanPathRankUnknown,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked outputs=%v, want fail-closed %v", got, want)
	}
}

func TestCleanPathRankAggregatesSelectedOccurrence(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60},
		{predecessor: 2, prefixScore: []int64{4}, gotoState: 60},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].CleanPathRank != CleanPathRankSelected {
		t.Fatalf("aggregated outputs=%+v, want one selected output", outputs)
	}
}

func TestCleanPathRankKeepsSamePathPrefixTieKnown(t *testing.T) {
	compact, head := newAmbiguousPrefixRankFixture(t)
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	got := cleanPathRankOutputStates(t, compact, outputs)
	want := map[StateID]CleanPathRankSelection{
		60: CleanPathRankSelected,
		61: CleanPathRankUnselected,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked outputs=%v, want %v", got, want)
	}
}

func TestCleanPathRankPrefixCapIsUnknown(t *testing.T) {
	compact, head := newAmbiguousPrefixRankFixture(t)
	compact.limits.MaxDerivations = 1
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range outputs {
		if output.CleanPathRank != CleanPathRankUnknown {
			t.Fatalf("capped ranked output=%+v, want unknown", output)
		}
	}
}

func TestCleanPathRankExternalPayloadIsUnknown(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60, external: true},
		{predecessor: 2, prefixScore: []int64{2}, gotoState: 61},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range outputs {
		if output.CleanPathRank != CleanPathRankUnknown {
			t.Fatalf("external ranked output=%+v, want unknown", output)
		}
	}
}

func TestExactExternalPayloadRanksOnlyLineageDestination(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60, external: true, exact: true},
		{predecessor: 2, prefixScore: []int64{2}, gotoState: 61},
	})
	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	gotScheduler := cleanPathRankOutputStates(t, compact, outputs)
	wantScheduler := map[StateID]CleanPathRankSelection{
		60: CleanPathRankUnknown,
		61: CleanPathRankUnknown,
	}
	if !reflect.DeepEqual(gotScheduler, wantScheduler) {
		t.Fatalf("scheduler ranks=%v, want %v", gotScheduler, wantScheduler)
	}
	gotDestination := make(map[StateID]CleanPathRankSelection, len(outputs))
	for _, output := range outputs {
		record, err := compact.node(output.Head.Node)
		if err != nil {
			t.Fatal(err)
		}
		gotDestination[record.state] = output.LineageDestinationRank
	}
	wantDestination := map[StateID]CleanPathRankSelection{
		60: CleanPathRankSelected,
		61: CleanPathRankUnselected,
	}
	if !reflect.DeepEqual(gotDestination, wantDestination) {
		t.Fatalf("destination ranks=%v, want %v", gotDestination, wantDestination)
	}
}

func TestCleanPathRankExternalPayloadSteadyStateDoesNotAllocate(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60, external: true},
		{predecessor: 2, prefixScore: []int64{2}, gotoState: 61},
	})
	paths, err := compact.popPaths(head.Node, 1)
	if err != nil {
		t.Fatal(err)
	}
	compact.markCleanProductionRank(paths)
	if got := testing.AllocsPerRun(100, func() {
		compact.markCleanProductionRank(paths)
	}); got != 0 {
		t.Fatalf("external clean path rank allocations/run=%f, want 0", got)
	}
}

func TestCleanPathRankSteadyStateDoesNotAllocate(t *testing.T) {
	compact, head := newAmbiguousPrefixRankFixture(t)
	paths, err := compact.popPaths(head.Node, 1)
	if err != nil {
		t.Fatal(err)
	}
	compact.markCleanProductionRank(paths)
	if got := testing.AllocsPerRun(100, func() {
		compact.markCleanProductionRank(paths)
	}); got != 0 {
		t.Fatalf("clean path rank allocations/run=%f, want 0", got)
	}
}

func TestCleanPathRankPanicRollsBackAndRetries(t *testing.T) {
	compact, head := newCleanPathRankFixture(t, []cleanPathRankBranch{
		{predecessor: 1, prefixScore: []int64{5}, gotoState: 60},
		{predecessor: 2, prefixScore: []int64{1, 2}, gotoState: 61},
	})
	base, ok := compact.tables.(*fakeTable)
	if !ok {
		t.Fatalf("fixture tables type=%T, want *fakeTable", compact.tables)
	}
	tables := &panicNthGotoTable{base: base, panicAt: 2}
	compact.tables = tables

	before, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	beforeBoundaries := cloneBoundaryMap(compact.boundaries)
	beforeWork := compact.Work()
	beforeArenas := [...]int{
		len(compact.nodes), len(compact.links), len(compact.subtrees),
		len(compact.children), len(compact.fields), len(compact.aliases),
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "reduction scratch panic" {
				t.Fatalf("recovered=%v, want reduction scratch panic", recovered)
			}
		}()
		_, _ = compact.ReduceOutputsInto(make([]ReductionOutput, 0, 2), head, 9, 0, ForkOrder{})
	}()
	after, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	afterArenas := [...]int{
		len(compact.nodes), len(compact.links), len(compact.subtrees),
		len(compact.children), len(compact.fields), len(compact.aliases),
	}
	if after != before || afterArenas != beforeArenas ||
		!reflect.DeepEqual(compact.boundaries.logicalMap(), beforeBoundaries) ||
		compact.Work() != beforeWork {
		t.Fatalf(
			"rank rollback drifted: stats=%+v/%+v arenas=%v/%v boundaries=%v/%v work=%+v/%+v",
			after, before, afterArenas, beforeArenas,
			compact.boundaries.logicalMap(), beforeBoundaries, compact.Work(), beforeWork,
		)
	}
	if compact.popScratch.busy || len(compact.popScratch.paths) != 0 ||
		len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 {
		t.Fatalf("rank rollback retained logical pop scratch: %+v", compact.popScratch)
	}
	if compact.reductionScratch.spilled || len(compact.reductionScratch.boundaries) != 0 ||
		len(compact.reductionScratch.boundaryByKey) != 0 {
		t.Fatalf("rank rollback retained logical reduction scratch: %+v", compact.reductionScratch)
	}
	assertTransactionJournalClean(t, compact)

	tables.calls = 0
	tables.panicAt = 0
	outputs, err := compact.ReduceOutputsInto(make([]ReductionOutput, 0, 2), head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	got := cleanPathRankOutputStates(t, compact, outputs)
	if got[60] != CleanPathRankSelected || got[61] != CleanPathRankUnselected {
		t.Fatalf("retry ranked outputs=%v", got)
	}
}

func BenchmarkCleanPathRankMultiPop(b *testing.B) {
	compact, head := newAmbiguousPrefixRankFixture(b)
	paths, err := compact.popPaths(head.Node, 1)
	if err != nil {
		b.Fatal(err)
	}
	compact.markCleanProductionRank(paths)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		compact.markCleanProductionRank(paths)
	}
}
