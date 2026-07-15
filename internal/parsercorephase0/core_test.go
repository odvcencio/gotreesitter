package parsercorephase0

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

func TestPinnedGoConflictAndReductionMetadata(t *testing.T) {
	wantConflict := []Action{
		{Type: ActionReduce, Symbol: 171, ChildCount: 1},
		{Type: ActionShift, State: 194},
	}
	wantReduce := Action{Type: ActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44}
	wantReduceCell := []Action{wantReduce, Action{Type: ActionReduce, Symbol: 171, ChildCount: 1}}
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 20, symbol: 4}: wantConflict,
			{state: 20, symbol: 6}: wantReduceCell,
		},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 121}: 101,
			{state: 1, symbol: 171}: 2,
		},
		aliases: map[productionKey][]Symbol{{productionID: 44, childCount: 1}: {229}},
	}
	core, err := New(tables, Limits{MaxPathsPerBoundary: 8, MaxEnumeration: 8})
	if err != nil {
		t.Fatal(err)
	}

	got, err := core.Actions(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantConflict) {
		t.Fatalf("Go cell (20,4) = %+v, want pinned conflict %+v", got, wantConflict)
	}
	conflictSeed, _ := core.Seed(20, 0)
	conflictHead, err := core.Shift(conflictSeed, 4, 1, Token{Symbol: 4, EndByte: 1}, ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	conflictPaths, err := core.Derivations(conflictHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflictPaths) != 1 || !conflictPaths[0].HasBranchOrder || conflictPaths[0].BranchOrder != 1 {
		t.Fatalf("authentic conflict shift order = %#v, want current order 1", conflictPaths)
	}

	reduceActions, err := core.Actions(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reduceActions, wantReduceCell) {
		t.Fatalf("Go reduction cell (20,6)/ParseActions[107] drifted: %+v", reduceActions)
	}
	if gotoState, err := tables.Goto(1, 121); err != nil || gotoState != 101 {
		t.Fatalf("lookupGoto(1,121) = %d, %v, want 101", gotoState, err)
	}

	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var wantOrder uint64
	var wantScore int64
	for i := 0; i < int(wantReduce.ChildCount); i++ {
		target := StateID(1)
		if i == int(wantReduce.ChildCount)-1 {
			target = 20
		}
		order := uint64(10 + i)
		delta := int64(i + 1)
		head, err = core.appendDiagnosticPayload(head, target, Token{
			Symbol: Symbol(1), StartByte: uint32(i), EndByte: uint32(i + 1),
		}, pathMeta{ScoreDelta: delta, BranchOrder: ForkOrder{Present: true, Value: order}})
		if err != nil {
			t.Fatal(err)
		}
		wantScore += delta
		wantOrder = order
	}

	frontier, err := core.Reduce(head, 6, 0, ForkOrder{})
	if err != nil {
		t.Fatalf("Reduce(real Go action): %v", err)
	}
	if len(frontier) != 1 {
		t.Fatalf("reduction frontier has %d boundaries, want 1", len(frontier))
	}
	head = frontier[0]
	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(paths[0].Payloads) != 1 {
		t.Fatalf("reduced derivations = %#v, want one collapsed payload", paths)
	}
	wantScore += int64(wantReduce.DynamicPrecedence)
	if paths[0].Score != wantScore || !paths[0].HasBranchOrder || paths[0].BranchOrder != wantOrder {
		t.Fatalf("path meta = score %d order %v, want score %d order %v", paths[0].Score, paths[0].BranchOrder, wantScore, wantOrder)
	}

	view, err := core.Subtree(paths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != wantReduce.Symbol || view.ProductionID != wantReduce.ProductionID || view.DynamicPrecedence != wantReduce.DynamicPrecedence {
		t.Fatalf("reduction identity = (%d,%d,%d), want (%d,%d,%d)", view.Symbol, view.ProductionID, view.DynamicPrecedence, wantReduce.Symbol, wantReduce.ProductionID, wantReduce.DynamicPrecedence)
	}
	if len(view.Fields) != 0 {
		t.Fatalf("fields = %#v, want pinned empty production fields", view.Fields)
	}
	if !slices.Equal(view.Aliases, []Symbol{229}) {
		t.Fatalf("aliases = %#v, want pinned [229]", view.Aliases)
	}

	// ParseActions[106].Actions[0] is the pinned real Go production-zero
	// reduction from the conflict cell above. Production zero has no alias
	// row; generated metadata represents that ordinary case as nil rather
	// than a child-count-sized zero slice.
	base := StateID(1)
	noAliasCore, err := New(tables, Limits{MaxPathsPerBoundary: 8, MaxEnumeration: 8})
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err := noAliasCore.Seed(base, 0)
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err = noAliasCore.appendDiagnosticPayload(noAliasHead, 20, Token{Symbol: 1, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	noAliasFrontier, err := noAliasCore.Reduce(noAliasHead, 4, 0, ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatalf("Reduce(real Go no-alias action): %v", err)
	}
	if len(noAliasFrontier) != 1 {
		t.Fatalf("no-alias reduction frontier has %d boundaries, want 1", len(noAliasFrontier))
	}
	noAliasPaths, err := noAliasCore.Derivations(noAliasFrontier[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(noAliasPaths) != 1 || len(noAliasPaths[0].Payloads) != 1 {
		t.Fatalf("no-alias derivations = %#v, want one collapsed payload", noAliasPaths)
	}
	noAliasView, err := noAliasCore.Subtree(noAliasPaths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(noAliasView.Aliases) != 0 {
		t.Fatalf("ordinary Go production aliases = %v, want none", noAliasView.Aliases)
	}
}

func TestFrontierEpochPreventsStaleBoundaryResurrection(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := core.boundaryKey(2, 1)
	oldHead, err := core.condense(oldKey, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	newHead, err := core.condense(core.boundaryKey(2, 1), linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := core.Stats(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentExactPaths != 1 {
		t.Fatalf("new epoch inherited %d exact paths, want 1", stats.CurrentExactPaths)
	}
	paths, err := core.Derivations(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].Score != 2 {
		t.Fatalf("new epoch derivations = %#v, want only score-2 path", paths)
	}
	oldPaths, err := core.Derivations(oldHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldPaths) != 1 || oldPaths[0].Score != 1 {
		t.Fatalf("persistent old head derivations = %#v, want score-1 path", oldPaths)
	}
	if _, err := core.condense(oldKey, linkInput{prev: seed.Node, payload: payload, scoreDelta: 3}); err == nil || !strings.Contains(err.Error(), "frontier mismatch") {
		t.Fatalf("stale boundary insertion error = %v, want frontier mismatch", err)
	}
}

func TestHeaderConvergencePreservesDistinctScoreAndOrderPaths(t *testing.T) {
	core := newTinyCore(t, 6)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(2, 1)
	head, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1, order: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2, order: ForkOrder{Present: true, Value: 7}})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1, order: ForkOrder{Present: true, Value: 8}})
	if err != nil {
		t.Fatal(err)
	}

	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	want := []Derivation{
		{Payloads: []SubtreeID{payload}, Score: 1, BranchOrder: 7, HasBranchOrder: true},
		{Payloads: []SubtreeID{payload}, Score: 2, BranchOrder: 7, HasBranchOrder: true},
		{Payloads: []SubtreeID{payload}, Score: 1, BranchOrder: 8, HasBranchOrder: true},
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("derivations = %#v, want exact score/order alternatives %#v", paths, want)
	}
	stats, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Links != 3 {
		t.Fatalf("physical/logical links = %d, want one O(1) adjacency record per alternative", stats.Links)
	}
}

func TestSharedBoundaryCapCountsExactPathsWithoutScorePartition(t *testing.T) {
	core := newTinyCore(t, 2)
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	key := core.boundaryKey(2, 1)
	head, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: -100})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 200}); err == nil || !strings.Contains(err.Error(), "exact-path cap") {
		t.Fatalf("third score-partitioned path error = %v, want shared exact-path cap", err)
	}
	stats, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentExactPaths != 2 {
		t.Fatalf("exact paths after declined insert = %d, want 2", stats.CurrentExactPaths)
	}
}

func TestDeclinedDiagnosticAppendIsTransactional(t *testing.T) {
	core := newTinyCore(t, 1)
	seed, _ := core.Seed(1, 0)
	head, err := core.appendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	before, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.appendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: 2, BranchOrder: ForkOrder{Present: true, Value: 9}}); err == nil {
		t.Fatal("second exact path unexpectedly bypassed the shared cap")
	}
	after, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("declined append mutated storage: before=%+v after=%+v", before, after)
	}
}

func TestExtraShiftWithZeroTargetRetainsCurrentState(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 1, symbol: 1}: {{Type: ActionShift, Extra: true}},
		},
	}
	core, err := New(tables, Limits{MaxPathsPerBoundary: 2, MaxEnumeration: 2})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := core.Seed(1, 0)
	head, err := core.Shift(seed, 1, 0, Token{Symbol: 1, EndByte: 1, Extra: true}, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := core.node(head.Node)
	if n.state != 1 {
		t.Fatalf("extra shift target state = %d, want current state 1", n.state)
	}
	before, _ := core.Stats(head)
	if _, err := core.Shift(head, 1, 0, Token{Symbol: 1, EndByte: 2, Extra: false}, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "disagrees") {
		t.Fatalf("extra mismatch error = %v", err)
	}
	after, _ := core.Stats(head)
	if after != before {
		t.Fatalf("extra mismatch mutated core: before=%+v after=%+v", before, after)
	}
}

func TestReductionOverExtrasDeclinesTransactionally(t *testing.T) {
	for _, childCount := range []uint8{0, 1} {
		t.Run(string(rune('0'+childCount))+"_children", func(t *testing.T) {
			tables := diagnosticReduceTable(childCount, 0)
			core, err := New(tables, Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
			if err != nil {
				t.Fatal(err)
			}
			head, _ := core.Seed(1, 0)
			if childCount > 0 {
				head, err = core.appendDiagnosticPayload(head, 2, Token{Symbol: 1, EndByte: 1}, pathMeta{})
				if err != nil {
					t.Fatal(err)
				}
			}
			head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 1, StartByte: 1, EndByte: 2, Extra: true}, pathMeta{})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := core.Stats(head)
			key := core.boundaryKey(3, 2)
			beforeBoundary := core.boundaries[key]
			if _, err := core.Reduce(head, 1, 0, ForkOrder{}); !IsDecline(err, DeclineExtras) {
				t.Fatalf("Reduce over trailing extra error = %v, want extras decline", err)
			}
			after, _ := core.Stats(head)
			if after != before || core.boundaries[key] != beforeBoundary {
				t.Fatalf("extras decline mutated core: before=%+v/%d after=%+v/%d", before, beforeBoundary, after, core.boundaries[key])
			}
		})
	}
}

func TestReduceScoreOverflowRollsBack(t *testing.T) {
	core, err := New(diagnosticReduceTable(1, 1), Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, err = core.appendDiagnosticPayload(head, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{ScoreDelta: math.MaxInt64})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := core.Stats(head)
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "score overflow") {
		t.Fatalf("Reduce overflow error = %v", err)
	}
	after, _ := core.Stats(head)
	if after != before {
		t.Fatalf("Reduce overflow did not roll back: before=%+v after=%+v", before, after)
	}
}

func TestReductionReturnsEveryDistinctGotoBoundary(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[tableCell]StateID{
			{state: 1, symbol: 2}: 4,
			{state: 2, symbol: 2}: 5,
		},
	}
	core, err := New(tables, Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := core.Seed(1, 0)
	right, _ := core.Seed(2, 0)
	left, err = core.appendDiagnosticPayload(left, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{BranchOrder: ForkOrder{Present: true, Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.appendDiagnosticPayload(right, 3, Token{Symbol: 1, EndByte: 1}, pathMeta{BranchOrder: ForkOrder{Present: true, Value: 2}})
	if err != nil {
		t.Fatal(err)
	}
	_ = left // merged is the immutable replacement containing both paths.
	frontier, err := core.Reduce(merged, 1, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(frontier) != 2 {
		t.Fatalf("reduction frontier has %d boundaries, want 2", len(frontier))
	}
	states := make([]StateID, 0, 2)
	for _, head := range frontier {
		n, _ := core.node(head.Node)
		states = append(states, n.state)
	}
	slices.Sort(states)
	if !slices.Equal(states, []StateID{4, 5}) {
		t.Fatalf("reduction frontier states = %v, want [4 5]", states)
	}
}

func TestCompactArenaRecordsRemainPointerFree(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"node":    reflect.TypeFor[nodeRecord](),
		"link":    reflect.TypeFor[linkRecord](),
		"subtree": reflect.TypeFor[subtreeRecord](),
	} {
		for i := 0; i < typ.NumField(); i++ {
			kind := typ.Field(i).Type.Kind()
			if kind == reflect.Pointer || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Interface || kind == reflect.String {
				t.Fatalf("%s record field %s is GC-visible %s", name, typ.Field(i).Name, kind)
			}
		}
	}
	if got := unsafe.Sizeof(nodeRecord{}); got > 32 {
		t.Fatalf("nodeRecord size = %d, want <= 32", got)
	}
	if got := unsafe.Sizeof(linkRecord{}); got > 32 {
		t.Fatalf("linkRecord size = %d, want <= 32", got)
	}
	if got := unsafe.Sizeof(subtreeRecord{}); got > 64 {
		t.Fatalf("subtreeRecord size = %d, want <= 64", got)
	}
}

func TestCycleAndOverflowDeclineFailClosed(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		n, _ := core.node(head.Node)
		core.links[n.firstLink-1].prev = head.Node // adversarial arena corruption
		if _, err := core.Derivations(head); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("Derivations cycle error = %v", err)
		}
	})

	t.Run("path_count", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxPathsPerBoundary: math.MaxUint64, MaxEnumeration: math.MaxUint64})
		seedA, _ := core.Seed(1, 0)
		seedB, _ := core.Seed(2, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		key := core.boundaryKey(3, 0)
		if _, err := core.condense(key, linkInput{prev: seedA.Node, payload: payload}); err != nil {
			t.Fatal(err)
		}
		b, _ := core.node(seedB.Node)
		b.pathCount = math.MaxUint64
		if _, err := core.condense(key, linkInput{prev: seedB.Node, payload: payload}); err == nil || !strings.Contains(err.Error(), "path count overflow") {
			t.Fatalf("path-count overflow error = %v", err)
		}
	})

	t.Run("score", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload, scoreDelta: math.MaxInt64})
		if err != nil {
			t.Fatal(err)
		}
		head, err = core.condense(core.boundaryKey(3, 0), linkInput{prev: head.Node, payload: payload, scoreDelta: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := core.Derivations(head); err == nil || !strings.Contains(err.Error(), "score overflow") {
			t.Fatalf("score overflow error = %v", err)
		}
	})

	t.Run("max_branch_order_is_scalar_not_arithmetic", func(t *testing.T) {
		core := newTinyCore(t, 2)
		seed, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		head, err := core.condense(core.boundaryKey(2, 0), linkInput{prev: seed.Node, payload: payload, order: ForkOrder{Present: true, Value: math.MaxUint64}})
		if err != nil {
			t.Fatal(err)
		}
		paths, err := core.Derivations(head)
		if err != nil || len(paths) != 1 || paths[0].BranchOrder != math.MaxUint64 {
			t.Fatalf("max scalar branch order = %#v, %v", paths, err)
		}
	})
}

func newTinyCore(t *testing.T, pathCap uint64) *Core {
	t.Helper()
	return newTinyCoreWithLimits(t, Limits{MaxPathsPerBoundary: pathCap, MaxEnumeration: pathCap})
}

func newTinyCoreWithLimits(t *testing.T, limits Limits) *Core {
	t.Helper()
	core, err := New(&fakeTable{}, limits)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func diagnosticReduceTable(childCount uint8, dynamicPrecedence int16) *fakeTable {
	return &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 1}: {{
				Type: ActionReduce, Symbol: 2, ChildCount: childCount, DynamicPrecedence: dynamicPrecedence,
			}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
}
