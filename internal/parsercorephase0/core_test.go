package parsercorephase0

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unsafe"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGoScannerBackedFullParseDeclinesBeforeCoreExecution(t *testing.T) {
	lang := grammars.GoLanguage()
	if lang.ExternalScanner == nil || len(lang.ExternalSymbols) == 0 {
		t.Fatal("test requires the certified Go external scanner attachment")
	}
	err := AdmitFullParse(lang, FullParseRequest{})
	if !IsDecline(err, DeclineExternalScanner) {
		t.Fatalf("AdmitFullParse error = %v, want external-scanner decline", err)
	}
	if !strings.Contains(err.Error(), "scanner/election") {
		t.Fatalf("decline %q does not identify the next integration seam", err)
	}
}

func TestRealGoDecodedConflictAndReductionMetadata(t *testing.T) {
	lang := grammars.GoLanguage()
	core, err := New(lang, Limits{MaxPathsPerBoundary: 8, MaxEnumeration: 8})
	if err != nil {
		t.Fatal(err)
	}

	wantConflict := []gts.ParseAction{
		{Type: gts.ParseActionReduce, Symbol: 171, ChildCount: 1},
		{Type: gts.ParseActionShift, State: 194},
	}
	if lang.LargeStateCount != 2 || lang.SmallParseTableMap[18] != 814 {
		t.Fatalf("Go sparse table identity drifted: large=%d state20-offset=%d", lang.LargeStateCount, lang.SmallParseTableMap[18])
	}
	if got := rawSparseActionIndex(t, lang, 20, 4); got != 106 {
		t.Fatalf("Go raw sparse action cell (20,4) = %d, want pinned index 106", got)
	}
	got, err := core.Actions(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantConflict) {
		t.Fatalf("Go cell (20,4) = %+v, want pinned conflict %+v", got, wantConflict)
	}
	if !reflect.DeepEqual(lang.ParseActions[106].Actions, wantConflict) {
		t.Fatalf("Go ParseActions[106] drifted: %+v", lang.ParseActions[106].Actions)
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

	wantReduce := gts.ParseAction{Type: gts.ParseActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44}
	wantReduceCell := []gts.ParseAction{
		wantReduce,
		{Type: gts.ParseActionReduce, Symbol: 171, ChildCount: 1},
	}
	if got := rawSparseActionIndex(t, lang, 20, 6); got != 107 {
		t.Fatalf("Go raw sparse action cell (20,6) = %d, want pinned index 107", got)
	}
	reduceActions, err := core.Actions(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reduceActions, wantReduceCell) || !reflect.DeepEqual(lang.ParseActions[107].Actions, wantReduceCell) {
		t.Fatalf("Go reduction cell (20,6)/ParseActions[107] drifted: %+v", reduceActions)
	}
	if lang.ParseTable[1][121] != 101 {
		t.Fatalf("Go raw GOTO cell (1,121) = %d, want pinned state 101", lang.ParseTable[1][121])
	}
	if gotoState, err := lookupGoto(lang, 1, 121); err != nil || gotoState != 101 {
		t.Fatalf("lookupGoto(1,121) = %d, %v, want 101", gotoState, err)
	}
	if span := lang.FieldMapSlices[44]; span != [2]uint16{0, 0} {
		t.Fatalf("Go production 44 field span = %v, want empty", span)
	}
	if got := lang.AliasSequences[44]; !slices.Equal(got, []gts.Symbol{229}) {
		t.Fatalf("Go production 44 aliases = %v, want pinned [229]", got)
	}

	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var wantOrder uint64
	var wantScore int64
	for i := 0; i < int(wantReduce.ChildCount); i++ {
		target := gts.StateID(1)
		if i == int(wantReduce.ChildCount)-1 {
			target = 20
		}
		order := uint64(10 + i)
		delta := int64(i + 1)
		head, err = core.AppendDiagnosticPayload(head, target, Token{
			Symbol: gts.Symbol(1), StartByte: uint32(i), EndByte: uint32(i + 1),
		}, PathMeta{ScoreDelta: delta, BranchOrder: ForkOrder{Present: true, Value: order}})
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
	if !slices.Equal(view.Aliases, []gts.Symbol{229}) {
		t.Fatalf("aliases = %#v, want pinned [229]", view.Aliases)
	}

	// ParseActions[106].Actions[0] is the pinned real Go production-zero
	// reduction from the conflict cell above. Production zero has no alias
	// row; generated metadata represents that ordinary case as nil rather
	// than a child-count-sized zero slice.
	if got := lang.AliasSequences[0]; len(got) != 0 {
		t.Fatalf("Go production 0 aliases = %v, want pinned empty row", got)
	}
	var base gts.StateID
	for state := gts.StateID(1); uint32(state) < lang.StateCount; state++ {
		gotoState, lookupErr := lookupGoto(lang, state, wantConflict[0].Symbol)
		if lookupErr == nil && gotoState != 0 {
			base = state
			break
		}
	}
	if base == 0 {
		t.Fatalf("Go grammar has no GOTO for pinned reduced symbol %d", wantConflict[0].Symbol)
	}
	noAliasCore, err := New(lang, Limits{MaxPathsPerBoundary: 8, MaxEnumeration: 8})
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err := noAliasCore.Seed(base, 0)
	if err != nil {
		t.Fatal(err)
	}
	noAliasHead, err = noAliasCore.AppendDiagnosticPayload(noAliasHead, 20, Token{Symbol: 1, EndByte: 1}, PathMeta{})
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
	head, err := core.AppendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, PathMeta{ScoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	before, err := core.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.AppendDiagnosticPayload(seed, 2, Token{Symbol: 1, EndByte: 1}, PathMeta{ScoreDelta: 2, BranchOrder: ForkOrder{Present: true, Value: 9}}); err == nil {
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
	lang := &gts.Language{
		StateCount: 4, TokenCount: 2, InitialState: 1,
		ParseTable: [][]uint16{{0, 0}, {0, 1}, {0, 0}, {0, 0}},
		ParseActions: []gts.ParseActionEntry{
			{},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, Extra: true, State: 0}}},
		},
	}
	core, err := New(lang, Limits{MaxPathsPerBoundary: 2, MaxEnumeration: 2})
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
			lang := diagnosticReduceLanguage(childCount, 0)
			core, err := New(lang, Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
			if err != nil {
				t.Fatal(err)
			}
			head, _ := core.Seed(1, 0)
			if childCount > 0 {
				head, err = core.AppendDiagnosticPayload(head, 2, Token{Symbol: 1, EndByte: 1}, PathMeta{})
				if err != nil {
					t.Fatal(err)
				}
			}
			head, err = core.AppendDiagnosticPayload(head, 3, Token{Symbol: 1, StartByte: 1, EndByte: 2, Extra: true}, PathMeta{})
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
	core, err := New(diagnosticReduceLanguage(1, 1), Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
	if err != nil {
		t.Fatal(err)
	}
	head, _ := core.Seed(1, 0)
	head, err = core.AppendDiagnosticPayload(head, 3, Token{Symbol: 1, EndByte: 1}, PathMeta{ScoreDelta: math.MaxInt64})
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
	lang := &gts.Language{
		StateCount: 6, TokenCount: 2, InitialState: 1,
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 0, 4}, // state 1 GOTO(nonterminal 2) -> state 4
			{0, 0, 5}, // state 2 GOTO(nonterminal 2) -> state 5
			{0, 1, 0}, // state 3 reduces on terminal 1
			{0, 0, 0},
			{0, 0, 0},
		},
		ParseActions: []gts.ParseActionEntry{
			{},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionReduce, Symbol: 2, ChildCount: 1}}},
		},
	}
	core, err := New(lang, Limits{MaxPathsPerBoundary: 4, MaxEnumeration: 4})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := core.Seed(1, 0)
	right, _ := core.Seed(2, 0)
	left, err = core.AppendDiagnosticPayload(left, 3, Token{Symbol: 1, EndByte: 1}, PathMeta{BranchOrder: ForkOrder{Present: true, Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.AppendDiagnosticPayload(right, 3, Token{Symbol: 1, EndByte: 1}, PathMeta{BranchOrder: ForkOrder{Present: true, Value: 2}})
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
	states := make([]gts.StateID, 0, 2)
	for _, head := range frontier {
		n, _ := core.node(head.Node)
		states = append(states, n.state)
	}
	slices.Sort(states)
	if !slices.Equal(states, []gts.StateID{4, 5}) {
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
	core, err := New(&gts.Language{
		StateCount: 16, TokenCount: 4, InitialState: 1,
		ParseTable: make([][]uint16, 16),
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func diagnosticReduceLanguage(childCount uint8, dynamicPrecedence int16) *gts.Language {
	return &gts.Language{
		StateCount: 5, TokenCount: 2, InitialState: 1,
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 0, 4}, // state 1 GOTO(nonterminal 2) -> state 4
			{0, 0, 0},
			{0, 1, 0}, // state 3 reduces on terminal 1
			{0, 0, 0},
		},
		ParseActions: []gts.ParseActionEntry{
			{},
			{Actions: []gts.ParseAction{{
				Type: gts.ParseActionReduce, Symbol: 2, ChildCount: childCount,
				DynamicPrecedence: dynamicPrecedence,
			}}},
		},
	}
}

// rawSparseActionIndex decodes the pinned generated table directly in the
// test, independently of Core.Actions/lookupActionIndex.
func rawSparseActionIndex(t *testing.T, lang *gts.Language, state gts.StateID, symbol gts.Symbol) uint16 {
	t.Helper()
	idx := int(state) - int(lang.LargeStateCount)
	if idx < 0 || idx >= len(lang.SmallParseTableMap) {
		t.Fatalf("state %d is not a sparse-table state", state)
	}
	pos := int(lang.SmallParseTableMap[idx])
	groups := int(lang.SmallParseTable[pos])
	pos++
	for group := 0; group < groups; group++ {
		if pos+1 >= len(lang.SmallParseTable) {
			t.Fatal("truncated raw sparse group")
		}
		value := lang.SmallParseTable[pos]
		count := int(lang.SmallParseTable[pos+1])
		pos += 2
		if pos+count > len(lang.SmallParseTable) {
			t.Fatal("truncated raw sparse symbols")
		}
		for _, got := range lang.SmallParseTable[pos : pos+count] {
			if got == uint16(symbol) {
				return value
			}
		}
		pos += count
	}
	return 0
}
