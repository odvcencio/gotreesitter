package parsercorephase0

import (
	"reflect"
	"slices"
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

func TestPopScratchAdjacencyFramesPreserveStablePathOrderAndDepth(t *testing.T) {
	compact, head := newReductionLinkCollisionFixture(t, false)
	paths, err := compact.popPaths(head.Node, 2)
	if err != nil || len(paths) != 2 {
		t.Fatalf("pop paths=%+v err=%v", paths, err)
	}
	if got := []int64{paths[0].score, paths[1].score}; !slices.Equal(got, []int64{2, 5}) {
		t.Fatalf("stable path scores=%v, want oldest-to-newest [2 5]", got)
	}
	if got := []uint64{paths[0].order.Value, paths[1].order.Value}; !slices.Equal(got, []uint64{7, 9}) {
		t.Fatalf("stable path orders=%v, want oldest-to-newest [7 9]", got)
	}
	if len(compact.popScratch.linkFrames) < 2 || cap(compact.popScratch.linkFrames[0]) < 2 || cap(compact.popScratch.linkFrames[1]) < 1 {
		t.Fatalf("recursion frames=%v, want distinct retained frames for two active depths", compact.popScratch.linkFrames)
	}
	outer := compact.popScratch.linkFrames[0][:1]
	inner := compact.popScratch.linkFrames[1][:1]
	if &outer[0] == &inner[0] {
		t.Fatal("active recursion depths alias one adjacency frame")
	}
	for depth, frame := range compact.popScratch.linkFrames {
		if len(frame) != 0 {
			t.Fatalf("finished recursion frame %d retained logical length %d", depth, len(frame))
		}
	}
}

func TestPopScratchAdjacencyFramesRejectCorruptionAndRetryCleanly(t *testing.T) {
	compact, head := newReductionLinkCollisionFixture(t, false)
	node, err := compact.node(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	originalNode := *node
	originalLinks := append([]linkRecord(nil), compact.links...)

	tests := []struct {
		name    string
		corrupt func()
		want    string
	}{
		{
			name: "short",
			corrupt: func() {
				node.linkCount = originalNode.linkCount + 1
			},
			want: "shorter than recorded link count",
		},
		{
			name: "out-of-range",
			corrupt: func() {
				node.linkCount = 1
				node.firstLink = uint32(len(compact.links) + 1)
			},
			want: "link adjacency out of range",
		},
		{
			name: "overlong-cycle",
			corrupt: func() {
				compact.links[originalNode.firstLink-1].next = LinkID(originalNode.firstLink)
			},
			want: "exceeds recorded link count or cycles",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			*node = originalNode
			copy(compact.links, originalLinks)
			test.corrupt()
			if _, err := compact.popPaths(head.Node, 2); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("corruption error=%v, want %q", err, test.want)
			}
			if compact.popScratch.busy || len(compact.popScratch.visiting) != 0 || len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 || len(compact.popScratch.revOrders) != 0 || len(compact.popScratch.trailing) != 0 || len(compact.popScratch.paths) != 0 {
				t.Fatalf("failed traversal retained logical scratch: %+v", compact.popScratch)
			}
			for depth, frame := range compact.popScratch.linkFrames {
				if len(frame) != 0 {
					t.Fatalf("failed traversal frame %d retained logical length %d", depth, len(frame))
				}
			}

			*node = originalNode
			copy(compact.links, originalLinks)
			paths, err := compact.popPaths(head.Node, 2)
			if err != nil || len(paths) != 2 || paths[0].score != 2 || paths[1].score != 5 {
				t.Fatalf("clean retry paths=%+v err=%v", paths, err)
			}
		})
	}
}

func TestNodeLinksIntoRejectsRecordedCountOutsideBounds(t *testing.T) {
	compact, head := newReductionLinkCollisionFixture(t, false)
	node, err := compact.node(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	tooMany := *node
	tooMany.linkCount = compact.limits.MaxLinksPerBoundary + 1
	if _, err := compact.nodeLinksInto(make([]linkRecord, 0, 4), tooMany); err == nil || !strings.Contains(err.Error(), "configured limit") {
		t.Fatalf("configured-limit error=%v", err)
	}
	overArena := *node
	overArena.linkCount = uint32(len(compact.links) + 1)
	compact.limits.MaxLinksPerBoundary = overArena.linkCount
	if _, err := compact.nodeLinksInto(make([]linkRecord, 0, 4), overArena); err == nil || !strings.Contains(err.Error(), "exceeds link arena") {
		t.Fatalf("arena-limit error=%v", err)
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

func TestPopScratchAdjacencyFramesDoNotAllocateAfterPriming(t *testing.T) {
	compact, head := newReductionLinkCollisionFixture(t, false)
	for range 4 {
		paths, err := compact.popPaths(head.Node, 2)
		if err != nil || len(paths) != 2 {
			t.Fatalf("prime paths=%+v err=%v", paths, err)
		}
	}
	var runErr error
	var pathCount int
	if allocs := testing.AllocsPerRun(1000, func() {
		var paths []popPath
		paths, runErr = compact.popPaths(head.Node, 2)
		pathCount = len(paths)
	}); allocs != 0 || runErr != nil || pathCount != 2 {
		t.Fatalf("steady adjacency pop allocs=%v paths=%d err=%v", allocs, pathCount, runErr)
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
	wantFrameCaps := make([]int, len(compact.popScratch.linkFrames))
	for depth, frame := range compact.popScratch.linkFrames {
		wantFrameCaps[depth] = cap(frame)
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
	if len(compact.popScratch.linkFrames) != len(wantFrameCaps) {
		t.Fatalf("reset adjacency frame count=%d want=%d", len(compact.popScratch.linkFrames), len(wantFrameCaps))
	}
	for depth, frame := range compact.popScratch.linkFrames {
		if len(frame) != 0 || cap(frame) != wantFrameCaps[depth] {
			t.Fatalf("reset adjacency frame %d len/cap=%d/%d want=0/%d", depth, len(frame), cap(frame), wantFrameCaps[depth])
		}
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
