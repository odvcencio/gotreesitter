package parsercorephase0

import (
	"reflect"
	"strings"
	"testing"
)

type reentrantGotoTable struct {
	base  *fakeTable
	core  *Core
	head  Head
	fired bool
}

func (r *reentrantGotoTable) Actions(state StateID, symbol Symbol) (ActionRow, error) {
	return r.base.Actions(state, symbol)
}

func (r *reentrantGotoTable) Goto(state StateID, symbol Symbol) (StateID, error) {
	if !r.fired {
		r.fired = true
		_, err := r.core.ReduceOutputs(r.head, 9, 0, ForkOrder{})
		return 0, err
	}
	return r.base.Goto(state, symbol)
}

func (r *reentrantGotoTable) ProductionFields(productionID uint16, childCount int) ([]FieldMapEntry, error) {
	return r.base.ProductionFields(productionID, childCount)
}

func (r *reentrantGotoTable) ProductionAliases(productionID uint16, childCount int) ([]Symbol, error) {
	return r.base.ProductionAliases(productionID, childCount)
}

func newPopScratchBranchFixture(t *testing.T) (*Core, Head) {
	t.Helper()
	compact, head := newReductionLinkCollisionFixture(t, false)
	extra, err := compact.appendSubtree(subtreeRecord{
		symbol: 12, startByte: 2, endByte: 3, extra: true, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.appendPrivate(3, 3, linkInput{
		prev: head.Node, payload: extra, scoreDelta: 4,
		order: ForkOrder{Present: true, Value: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	return compact, head
}

func TestPopScratchPathsAreIndependentWithinOneEnumeration(t *testing.T) {
	compact, head := newPopScratchBranchFixture(t)
	paths, err := compact.popPaths(head.Node, 2)
	if err != nil || len(paths) != 2 {
		t.Fatalf("pop paths=%+v err=%v", paths, err)
	}
	if len(paths[0].children) == 0 || len(paths[1].children) == 0 || &paths[0].children[0] == &paths[1].children[0] {
		t.Fatalf("child buffers alias within one result: %+v", paths)
	}
	if len(paths[0].trailing) != 1 || len(paths[1].trailing) != 1 || &paths[0].trailing[0] == &paths[1].trailing[0] {
		t.Fatalf("trailing buffers alias within one result: %+v", paths)
	}
	want := append([]popPath(nil), paths...)
	for index := range want {
		want[index].children = append([]SubtreeID(nil), paths[index].children...)
		want[index].trailing = append([]pathPayload(nil), paths[index].trailing...)
	}

	// Results are private and ephemeral across serial calls. ReduceOutputs
	// consumes them before calling popPaths again; the next result must still
	// reproduce every value and preserve per-slot independence.
	again, err := compact.popPaths(head.Node, 2)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("successive pop paths=%+v want=%+v err=%v", again, want, err)
	}
	if &again[0].children[0] == &again[1].children[0] || &again[0].trailing[0] == &again[1].trailing[0] {
		t.Fatalf("successive result aliases slots: %+v", again)
	}
}

func TestPopScratchCapFailureClearsLogicalStateAndRetries(t *testing.T) {
	compact, head := newPopScratchBranchFixture(t)
	compact.limits.MaxPopPaths = 1
	if _, err := compact.popPaths(head.Node, 2); err == nil || !strings.Contains(err.Error(), "pop enumeration cap") {
		t.Fatalf("cap error=%v", err)
	}
	if compact.popScratch.busy || len(compact.popScratch.visiting) != 0 || len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 || len(compact.popScratch.revOrders) != 0 || len(compact.popScratch.trailing) != 0 || len(compact.popScratch.paths) != 0 {
		t.Fatalf("failed pop retained logical scratch: %+v", compact.popScratch)
	}
	compact.limits.MaxPopPaths = 2
	paths, err := compact.popPaths(head.Node, 2)
	if err != nil || len(paths) != 2 {
		t.Fatalf("retry paths=%+v err=%v", paths, err)
	}
}

func TestPopScratchRejectsReentrantReductionAndRollsBackOuterCall(t *testing.T) {
	base := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 2}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
	tables := &reentrantGotoTable{base: base}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.appendDiagnosticPayload(seed, 2, Token{Symbol: 10, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.appendDiagnosticPayload(first, 3, Token{Symbol: 11, StartByte: 1, EndByte: 2}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	tables.core = compact
	tables.head = head

	before, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	beforeBoundaries := cloneBoundaryMap(compact.boundaries)
	beforeWork := compact.Work()
	if _, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "reentrant reduction") {
		t.Fatalf("reentrant reduction error=%v", err)
	}
	after, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || !reflect.DeepEqual(compact.boundaries, beforeBoundaries) || compact.Work() != beforeWork {
		t.Fatalf("reentrant rollback mutated core: stats=%+v/%+v boundaries=%v/%v work=%+v/%+v", after, before, compact.boundaries, beforeBoundaries, compact.Work(), beforeWork)
	}
	if compact.popScratch.busy || len(compact.popScratch.visiting) != 0 || len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 || len(compact.popScratch.revOrders) != 0 || len(compact.popScratch.trailing) != 0 || len(compact.popScratch.paths) != 0 {
		t.Fatalf("reentrant rollback retained logical scratch: %+v", compact.popScratch)
	}
	assertTransactionJournalClean(t, compact)

	outputs, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil || len(outputs) != 1 {
		t.Fatalf("retry outputs=%+v err=%v", outputs, err)
	}
}

func TestPopScratchZeroChildSteadyStateDoesNotAllocate(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 7)
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := compact.popPaths(head.Node, 0); err != nil {
			t.Fatal(err)
		}
	}
	var runErr error
	if allocs := testing.AllocsPerRun(1000, func() {
		_, runErr = compact.popPaths(head.Node, 0)
	}); allocs != 0 || runErr != nil {
		t.Fatalf("zero-child steady pop allocs=%v err=%v", allocs, runErr)
	}
}

func TestResetClearsPopScratchAndRetainsCapacity(t *testing.T) {
	compact, head := newPopScratchBranchFixture(t)
	paths, err := compact.popPaths(head.Node, 2)
	if err != nil || len(paths) != 2 {
		t.Fatalf("pop paths=%+v err=%v", paths, err)
	}
	wantCaps := [...]int{
		cap(compact.popScratch.rev), cap(compact.popScratch.revScores),
		cap(compact.popScratch.revOrders), cap(compact.popScratch.trailing),
		cap(compact.popScratch.paths), cap(compact.popScratch.paths[0].children),
		cap(compact.popScratch.paths[0].trailing),
	}
	if compact.popScratch.visiting == nil {
		t.Fatal("pop visiting map was not initialized")
	}
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if compact.popScratch.busy || len(compact.popScratch.visiting) != 0 || len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 || len(compact.popScratch.revOrders) != 0 || len(compact.popScratch.trailing) != 0 || len(compact.popScratch.paths) != 0 {
		t.Fatalf("reset retained logical pop scratch: %+v", compact.popScratch)
	}
	retained := compact.popScratch.paths[:1][0]
	gotCaps := [...]int{
		cap(compact.popScratch.rev), cap(compact.popScratch.revScores),
		cap(compact.popScratch.revOrders), cap(compact.popScratch.trailing),
		cap(compact.popScratch.paths), cap(retained.children), cap(retained.trailing),
	}
	if gotCaps != wantCaps || compact.popScratch.visiting == nil {
		t.Fatalf("reset pop capacities=%v want=%v map_nil=%t", gotCaps, wantCaps, compact.popScratch.visiting == nil)
	}
}
