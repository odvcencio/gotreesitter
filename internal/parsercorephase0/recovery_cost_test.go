package parsercorephase0

import (
	"errors"
	"math/rand"
	"testing"
)

// fakeRecoveryCostSource is a minimal, in-memory RecoveryCostSource backing
// hand-built and generated fixtures. It never mutates published entries,
// matching the immutability RecoveryCostSource documents.
type fakeRecoveryCostSource map[SubtreeID]RecoveryCostNode

func (s fakeRecoveryCostSource) RecoveryCostNode(id SubtreeID) (RecoveryCostNode, error) {
	node, ok := s[id]
	if !ok {
		return RecoveryCostNode{}, ErrRecoveryCostNodeMissing
	}
	return node, nil
}

// recoveryCostSpec builds a fakeRecoveryCostSource fixture from a small tree
// literal, assigning sequential one-based SubtreeIDs in publish order
// (children before parents, matching compact's own append-only arena
// discipline).
type recoveryCostSpec struct {
	symbol   Symbol
	extra    bool
	missing  bool
	start    uint32
	end      uint32
	startRow uint32
	endRow   uint32
	aliases  []Symbol
	children []*recoveryCostSpec
}

func publishRecoveryCostSpec(src fakeRecoveryCostSource, next *SubtreeID, spec *recoveryCostSpec) SubtreeID {
	children := make([]SubtreeID, 0, len(spec.children))
	for _, child := range spec.children {
		children = append(children, publishRecoveryCostSpec(src, next, child))
	}
	*next++
	id := *next
	src[id] = RecoveryCostNode{
		Symbol:    spec.symbol,
		Extra:     spec.extra,
		Missing:   spec.missing,
		StartByte: spec.start,
		EndByte:   spec.end,
		StartRow:  spec.startRow,
		EndRow:    spec.endRow,
		Children:  children,
		Aliases:   append([]Symbol(nil), spec.aliases...),
	}
	return id
}

func newRecoveryCostFixture(spec *recoveryCostSpec) (fakeRecoveryCostSource, SubtreeID) {
	src := fakeRecoveryCostSource{}
	var next SubtreeID
	root := publishRecoveryCostSpec(src, &next, spec)
	return src, root
}

const ordinarySymbol Symbol = 5

func visibleSymbols() []SelectedSymbolPolicy {
	symbols := make([]SelectedSymbolPolicy, 8)
	symbols[ordinarySymbol] = SelectedSymbolPolicy{Visible: true, Named: true}
	symbols[6] = SelectedSymbolPolicy{Visible: false, Named: false}
	return symbols
}

func TestRecoveryNodeErrorCostCleanTreeIsZero(t *testing.T) {
	spec := &recoveryCostSpec{
		symbol: ordinarySymbol, start: 0, end: 10, endRow: 1,
		children: []*recoveryCostSpec{
			{symbol: ordinarySymbol, start: 0, end: 4},
			{symbol: ordinarySymbol, start: 4, end: 10, endRow: 1},
		},
	}
	src, root := newRecoveryCostFixture(spec)
	got, err := RecoveryNodeErrorCost(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("clean tree cost = %d, want 0", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountIncludesVisibleDescendants(t *testing.T) {
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol: ordinarySymbol,
		children: []*recoveryCostSpec{
			{symbol: ordinarySymbol},
			{symbol: 6, children: []*recoveryCostSpec{{symbol: ordinarySymbol}}},
			{symbol: RecoveryErrorSymbol},
		},
	})
	got, err := RecoveryNodeVisibleSubtreeCount(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Fatalf("visible subtree count=%d, want root, two ordinary nodes, and ERROR", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountAppliesProductionAliases(t *testing.T) {
	symbols := visibleSymbols()
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol:  ordinarySymbol,
		aliases: []Symbol{7, 0},
		children: []*recoveryCostSpec{
			{symbol: 6},
			{symbol: 6},
		},
	})
	got, err := RecoveryNodeVisibleSubtreeCount(symbols, src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("alias-aware visible subtree count=%d, want visible root and aliased first child", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountIgnoresAliasOnExtraChild(t *testing.T) {
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol:   6,
		aliases:  []Symbol{7},
		children: []*recoveryCostSpec{{symbol: 6, extra: true}},
	})
	got, err := RecoveryNodeVisibleSubtreeCount(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("extra-child alias visible count=%d, want 0", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountIncludesHiddenErrorRepeat(t *testing.T) {
	src, root := newRecoveryCostFixture(&recoveryCostSpec{symbol: RecoveryErrorRepeatSymbol})
	got, err := RecoveryNodeVisibleSubtreeCount(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("ERROR_REPEAT visible count=%d, want progress count 1", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountDoesNotCountNestedErrorRepeatWrapper(t *testing.T) {
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol: ordinarySymbol,
		children: []*recoveryCostSpec{{
			symbol:   RecoveryErrorRepeatSymbol,
			children: []*recoveryCostSpec{{symbol: ordinarySymbol}},
		}},
	})
	got, err := RecoveryNodeVisibleSubtreeCount(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("nested ERROR_REPEAT visible count=%d, want visible root and descendant", got)
	}
}

func TestRecoveryNodeVisibleSubtreeCountRejectsMalformedAliasRow(t *testing.T) {
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol:   ordinarySymbol,
		aliases:  []Symbol{7, 6},
		children: []*recoveryCostSpec{{symbol: ordinarySymbol}},
	})
	if _, err := RecoveryNodeVisibleSubtreeCount(visibleSymbols(), src, root); err == nil {
		t.Fatal("malformed recovery alias row unexpectedly passed")
	}
}

// TestRecoveryNodeErrorCostChildlessErrorLeafRule pins
// parser_recover_c.go:1243-1246: a childless ERROR node contributes exactly
// 0 to its PARENT's accumulated cost (its bytes are charged once via the
// enclosing ERROR node's own span), but the same childless ERROR node
// queried directly (as a root, not through a parent's child loop) still
// charges its own RecoveryCostPerRecovery+bytes+rows -- matching the C
// error_repeat wrapper around an invisible token
// (parser_recover_c.go:1223-1233).
func TestRecoveryNodeErrorCostChildlessErrorLeafRule(t *testing.T) {
	leaf := &recoveryCostSpec{symbol: RecoveryErrorSymbol, start: 3, end: 7, startRow: 1, endRow: 2}

	src, leafID := newRecoveryCostFixture(leaf)
	directCost, err := RecoveryNodeErrorCost(visibleSymbols(), src, leafID)
	if err != nil {
		t.Fatal(err)
	}
	wantDirect := uint32(RecoveryCostPerRecovery + RecoveryCostPerSkippedChar*4 + RecoveryCostPerSkippedLine*1)
	if directCost != wantDirect {
		t.Fatalf("childless ERROR leaf queried directly = %d, want %d", directCost, wantDirect)
	}

	parent := &recoveryCostSpec{
		symbol: ordinarySymbol, start: 0, end: 20, endRow: 3,
		children: []*recoveryCostSpec{
			{symbol: ordinarySymbol, start: 0, end: 3},
			{symbol: RecoveryErrorSymbol, start: 3, end: 7, startRow: 1, endRow: 2},
			{symbol: ordinarySymbol, start: 7, end: 20, startRow: 2, endRow: 3},
		},
	}
	src2, root := newRecoveryCostFixture(parent)
	parentCost, err := RecoveryNodeErrorCost(visibleSymbols(), src2, root)
	if err != nil {
		t.Fatal(err)
	}
	if parentCost != 0 {
		t.Fatalf("parent cost through a childless ERROR child = %d, want 0 (leaf skipped)", parentCost)
	}
}

func TestRecoveryNodeErrorCostMissingLeaf(t *testing.T) {
	spec := &recoveryCostSpec{symbol: ordinarySymbol, missing: true, start: 5, end: 5, startRow: 1, endRow: 1}
	src, root := newRecoveryCostFixture(spec)
	got, err := RecoveryNodeErrorCost(visibleSymbols(), src, root)
	if err != nil {
		t.Fatal(err)
	}
	want := uint32(RecoveryCostPerMissingTree + RecoveryCostPerRecovery)
	if got != want {
		t.Fatalf("missing leaf cost = %d, want %d", got, want)
	}
}

// TestRecoveryNodeErrorCostErrorNodeSkippedChildren pins the
// visible/invisible/extra child accounting inside the ERROR branch
// (parser_recover_c.go:1249-1259).
func TestRecoveryNodeErrorCostErrorNodeSkippedChildren(t *testing.T) {
	symbols := visibleSymbols()
	// One visible skipped child (+100), one extra child (skipped
	// entirely, +0), one invisible child with two visible grandchildren
	// (+100*2), spanning 6 bytes and 2 rows.
	spec := &recoveryCostSpec{
		symbol: RecoveryErrorSymbol, start: 0, end: 6, startRow: 0, endRow: 2,
		children: []*recoveryCostSpec{
			{symbol: ordinarySymbol, start: 0, end: 1},
			{symbol: 6, extra: true, start: 1, end: 2},
			{
				symbol: 6, start: 2, end: 6, endRow: 2,
				children: []*recoveryCostSpec{
					{symbol: ordinarySymbol, start: 2, end: 4},
					{symbol: ordinarySymbol, start: 4, end: 6, endRow: 2},
				},
			},
		},
	}
	src, root := newRecoveryCostFixture(spec)
	got, err := RecoveryNodeErrorCost(symbols, src, root)
	if err != nil {
		t.Fatal(err)
	}
	want := uint32(RecoveryCostPerSkippedTree /* visible child */ +
		RecoveryCostPerSkippedTree*2 /* invisible wrapper's two visible grandchildren */ +
		RecoveryCostPerRecovery + RecoveryCostPerSkippedChar*6 + RecoveryCostPerSkippedLine*2)
	if got != want {
		t.Fatalf("ERROR node cost = %d, want %d", got, want)
	}
}

func TestRecoveryNodeErrorCostUsesAliasedGrandchildVisibility(t *testing.T) {
	symbols := visibleSymbols()
	src, root := newRecoveryCostFixture(&recoveryCostSpec{
		symbol: RecoveryErrorSymbol,
		children: []*recoveryCostSpec{{
			symbol:   6,
			aliases:  []Symbol{7},
			children: []*recoveryCostSpec{{symbol: 6}},
		}},
	})
	got, err := RecoveryNodeErrorCost(symbols, src, root)
	if err != nil {
		t.Fatal(err)
	}
	want := uint32(RecoveryCostPerSkippedTree + RecoveryCostPerRecovery)
	if got != want {
		t.Fatalf("alias-aware ERROR cost=%d, want %d", got, want)
	}
}

func TestRecoveryErrorRegionCostPricesUnpublishedOpenNode(t *testing.T) {
	symbols := visibleSymbols()
	src, child := newRecoveryCostFixture(&recoveryCostSpec{
		symbol: ordinarySymbol, start: 2, end: 9, startRow: 1, endRow: 3,
	})
	got, err := RecoveryErrorRegionCost(
		symbols, src, nil,
		2, 1, 9, 3,
		[]SubtreeID{child},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := uint32(RecoveryCostPerSkippedTree + RecoveryCostPerRecovery +
		RecoveryCostPerSkippedChar*7 + RecoveryCostPerSkippedLine*2)
	if got != want {
		t.Fatalf("open ERROR region cost=%d, want %d", got, want)
	}
}

func TestRecoverySymbolVisible(t *testing.T) {
	symbols := visibleSymbols()
	if !RecoverySymbolVisible(symbols, RecoveryErrorSymbol) {
		t.Fatal("RecoveryErrorSymbol must always be visible, even out of table range")
	}
	if !RecoverySymbolVisible(symbols, ordinarySymbol) {
		t.Fatal("symbol marked Visible in the table must report visible")
	}
	if RecoverySymbolVisible(symbols, 6) {
		t.Fatal("symbol marked not-Visible in the table must report not visible")
	}
	if RecoverySymbolVisible(symbols, Symbol(len(symbols)+50)) {
		t.Fatal("out-of-range ordinary symbol must report not visible")
	}
}

func TestRecoveryCostSourceMissingNodeErrors(t *testing.T) {
	src := fakeRecoveryCostSource{}
	_, err := RecoveryNodeErrorCost(nil, src, 1)
	if !errors.Is(err, ErrRecoveryCostNodeMissing) {
		t.Fatalf("err = %v, want wrapped ErrRecoveryCostNodeMissing", err)
	}
}

// TestRecoveryNodeErrorCostMemoSkipsChildlessErrorLeaf pins the memo-shape
// consequence of the childless-ERROR-leaf rule: a parent's skip-check reads
// the leaf's Symbol/Children directly without recursing into it, so the leaf
// itself is never memoized, even though it correctly contributes 0 to the
// parent's memoized cost.
func TestRecoveryNodeErrorCostMemoSkipsChildlessErrorLeaf(t *testing.T) {
	leaf := &recoveryCostSpec{symbol: RecoveryErrorSymbol, start: 3, end: 7}
	parent := &recoveryCostSpec{
		symbol:   ordinarySymbol,
		start:    0,
		end:      7,
		children: []*recoveryCostSpec{leaf},
	}
	src, root := newRecoveryCostFixture(parent)
	leafID := SubtreeID(1) // published first, children-before-parent order.

	var memo RecoveryCostMemo
	cost, err := RecoveryNodeErrorCostMemo(visibleSymbols(), src, &memo, root)
	if err != nil {
		t.Fatal(err)
	}
	if cost != 0 {
		t.Fatalf("parent cost = %d, want 0 (childless ERROR child skipped)", cost)
	}
	if _, ok := memo.lookup(leafID); ok {
		t.Fatal("a childless ERROR leaf reached only via a parent's skip-check must not be memoized")
	}
	if _, ok := memo.lookup(root); !ok {
		t.Fatal("the root itself must be memoized")
	}
}

func TestRecoveryCostMemoGrowGetResetLen(t *testing.T) {
	var memo RecoveryCostMemo
	if memo.Len() != 0 {
		t.Fatalf("zero value Len() = %d, want 0", memo.Len())
	}
	if _, ok := memo.lookup(3); ok {
		t.Fatal("lookup on empty memo must miss")
	}
	memo.store(3, 42)
	if got, ok := memo.lookup(3); !ok || got != 42 {
		t.Fatalf("lookup(3) = (%d, %v), want (42, true)", got, ok)
	}
	if memo.Len() < 3 {
		t.Fatalf("Len() = %d after storing id 3, want >= 3", memo.Len())
	}
	if _, ok := memo.lookup(1); ok {
		t.Fatal("lookup on a never-stored id below the grown length must miss")
	}
	memo.Reset()
	if _, ok := memo.lookup(3); ok {
		t.Fatal("lookup after Reset must miss")
	}
	if memo.Len() < 3 {
		t.Fatal("Reset must retain capacity")
	}
	// id == 0 (the compact nil-child sentinel) is a documented no-op.
	memo.store(0, 99)
	if _, ok := memo.lookup(0); ok {
		t.Fatal("id 0 must never be stored")
	}
	// A nil *RecoveryCostMemo behaves like an always-miss, always-discard
	// memo (RecoveryNodeErrorCost passes nil for the unmemoized entry
	// point).
	var nilMemo *RecoveryCostMemo
	if _, ok := nilMemo.lookup(1); ok {
		t.Fatal("nil memo lookup must miss")
	}
	nilMemo.store(1, 1)
	nilMemo.Reset()
	if nilMemo.Len() != 0 {
		t.Fatal("nil memo Len() must be 0")
	}
}

// TestRecoveryNodeErrorCostMemoMatchesUnmemoized runs a model-based property
// check across many generated tree shapes: the memoized and unmemoized cost
// entry points must return identical costs, and a warm memo (already
// populated by an earlier full walk) must return the same cost as one primed
// fresh, node for node.
func TestRecoveryNodeErrorCostMemoMatchesUnmemoized(t *testing.T) {
	symbols := visibleSymbols()
	random := rand.New(rand.NewSource(0x7265_636f))
	for trial := 0; trial < 500; trial++ {
		src, root, ids := genRecoveryCostTree(random)

		unmemoized, err := RecoveryNodeErrorCost(symbols, src, root)
		if err != nil {
			t.Fatalf("trial %d: unmemoized: %v", trial, err)
		}

		var freshMemo RecoveryCostMemo
		memoized, err := RecoveryNodeErrorCostMemo(symbols, src, &freshMemo, root)
		if err != nil {
			t.Fatalf("trial %d: memoized: %v", trial, err)
		}
		if memoized != unmemoized {
			t.Fatalf("trial %d: memoized root cost = %d, want %d (unmemoized)", trial, memoized, unmemoized)
		}

		// Every individual node's memoized cost must equal its own
		// unmemoized cost too (node-for-node parity), not just the root.
		//
		// Not every id is necessarily present in freshMemo after the root
		// walk: a childless ERROR leaf reached only through a parent's
		// skip-check (the rule TestRecoveryNodeErrorCostChildlessErrorLeafRule
		// pins) is never recursed into, so it is never memoized either --
		// exactly mirroring cNodeErrorCostLangWithScratch, which also never
		// calls itself (and so never populates scratch.cErrorCost) for a
		// child it skips. When present, the memoized value must still match.
		for _, id := range ids {
			want, err := RecoveryNodeErrorCost(symbols, src, id)
			if err != nil {
				t.Fatalf("trial %d: unmemoized node %d: %v", trial, id, err)
			}
			if gotWarm, ok := freshMemo.lookup(id); ok && gotWarm != want {
				t.Fatalf("trial %d: warm memo cost for node %d = %d, want %d", trial, id, gotWarm, want)
			}
			// A second, independent memoized call (cold memo) must agree too.
			var coldMemo RecoveryCostMemo
			gotCold, err := RecoveryNodeErrorCostMemo(symbols, src, &coldMemo, id)
			if err != nil {
				t.Fatalf("trial %d: cold memo node %d: %v", trial, id, err)
			}
			if gotCold != want {
				t.Fatalf("trial %d: cold memo cost for node %d = %d, want %d", trial, id, gotCold, want)
			}
		}
	}
}

// genRecoveryCostTree builds a small random compact subtree (bounded depth
// and fan-out, a mix of ordinary/extra/missing/ERROR nodes and byte/row
// spans) and returns its source, root id, and every published id in publish
// (children-before-parents) order.
func genRecoveryCostTree(random *rand.Rand) (fakeRecoveryCostSource, SubtreeID, []SubtreeID) {
	src := fakeRecoveryCostSource{}
	var next SubtreeID
	var ids []SubtreeID
	var build func(depth int) SubtreeID
	build = func(depth int) SubtreeID {
		childCount := 0
		if depth < 4 {
			switch {
			case random.Intn(3) == 0:
				childCount = 1 + random.Intn(3)
			}
		}
		isError := depth > 0 && random.Intn(6) == 0
		missing := childCount == 0 && !isError && random.Intn(8) == 0
		sym := ordinarySymbol
		switch {
		case isError:
			sym = RecoveryErrorSymbol
		case random.Intn(4) == 0:
			sym = Symbol(6) // invisible ordinary symbol
		}
		extra := childCount == 0 && random.Intn(5) == 0

		startByte := uint32(random.Intn(30))
		startRow := uint32(random.Intn(4))
		children := make([]SubtreeID, 0, childCount)
		endByte, endRow := startByte, startRow
		for i := 0; i < childCount; i++ {
			childID := build(depth + 1)
			children = append(children, childID)
			child := src[childID]
			if child.EndByte > endByte {
				endByte = child.EndByte
			}
			if child.EndRow > endRow {
				endRow = child.EndRow
			}
		}
		if childCount == 0 {
			endByte = startByte + uint32(random.Intn(5))
			endRow = startRow + uint32(random.Intn(2))
		}

		next++
		id := next
		src[id] = RecoveryCostNode{
			Symbol: sym, Extra: extra, Missing: missing,
			StartByte: startByte, EndByte: endByte, StartRow: startRow, EndRow: endRow,
			Children: children,
		}
		ids = append(ids, id)
		return id
	}
	root := build(0)
	return src, root, ids
}

func TestRecoveryVersionStatusPausedAddsSkippedTreeCost(t *testing.T) {
	clean := RecoveryVersionStatus(100, false, 3, 0, false)
	if clean.Cost != 100 || clean.IsInError {
		t.Fatalf("clean status = %+v", clean)
	}
	paused := RecoveryVersionStatus(100, true, 3, 0, false)
	if paused.Cost != 100+RecoveryCostPerSkippedTree {
		t.Fatalf("paused cost = %d, want %d", paused.Cost, 100+RecoveryCostPerSkippedTree)
	}
	if !paused.IsInError {
		t.Fatal("a paused status must report IsInError")
	}
	openRecovery := RecoveryVersionStatus(0, false, 0, 0, true)
	if !openRecovery.IsInError {
		t.Fatal("hasOpenRecovery must set IsInError even when not paused")
	}
}

func TestRecoveryCompareVersionsKnownCases(t *testing.T) {
	cases := []struct {
		name string
		a, b RecoveryErrorStatus
		want RecoveryComparison
	}{
		{
			name: "clean beats in-error regardless of cost when clean is cheaper",
			a:    RecoveryErrorStatus{Cost: 10, IsInError: false},
			b:    RecoveryErrorStatus{Cost: 999, IsInError: true},
			want: RecoveryComparisonTakeLeft,
		},
		{
			name: "clean but pricier than an in-error rival still only prefers left",
			a:    RecoveryErrorStatus{Cost: 999, IsInError: false},
			b:    RecoveryErrorStatus{Cost: 10, IsInError: true},
			want: RecoveryComparisonPreferLeft,
		},
		{
			name: "small cost gap within the band prefers the cheaper side",
			a:    RecoveryErrorStatus{Cost: 100, NodeCount: 0, IsInError: true},
			b:    RecoveryErrorStatus{Cost: 200, NodeCount: 0, IsInError: true},
			want: RecoveryComparisonPreferLeft,
		},
		{
			name: "large cost gap beyond the node-scaled band takes the cheaper side",
			a:    RecoveryErrorStatus{Cost: 100, NodeCount: 0, IsInError: true},
			b:    RecoveryErrorStatus{Cost: 100 + recoveryMaxCostDifference + 1, NodeCount: 0, IsInError: true},
			want: RecoveryComparisonTakeLeft,
		},
		{
			name: "equal cost falls through to dynamic precedence",
			a:    RecoveryErrorStatus{Cost: 50, DynPrec: 2, IsInError: true},
			b:    RecoveryErrorStatus{Cost: 50, DynPrec: 1, IsInError: true},
			want: RecoveryComparisonPreferLeft,
		},
		{
			name: "fully equal is None",
			a:    RecoveryErrorStatus{Cost: 50, DynPrec: 1, NodeCount: 4, IsInError: true},
			b:    RecoveryErrorStatus{Cost: 50, DynPrec: 1, NodeCount: 9, IsInError: true},
			want: RecoveryComparisonNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RecoveryCompareVersions(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("RecoveryCompareVersions(%+v, %+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestRecoveryCompareVersionsMirrorsOnSwap is a property test: swapping a and
// b must mirror TakeLeft<->TakeRight and PreferLeft<->PreferRight, and leave
// None fixed, for any generated pair.
func TestRecoveryCompareVersionsMirrorsOnSwap(t *testing.T) {
	random := rand.New(rand.NewSource(0x6d6972_726f72))
	mirror := map[RecoveryComparison]RecoveryComparison{
		RecoveryComparisonTakeLeft:    RecoveryComparisonTakeRight,
		RecoveryComparisonPreferLeft:  RecoveryComparisonPreferRight,
		RecoveryComparisonNone:        RecoveryComparisonNone,
		RecoveryComparisonPreferRight: RecoveryComparisonPreferLeft,
		RecoveryComparisonTakeRight:   RecoveryComparisonTakeLeft,
	}
	for trial := 0; trial < 2000; trial++ {
		a := RecoveryErrorStatus{
			Cost: uint32(random.Intn(3000)), NodeCount: random.Intn(20),
			DynPrec: random.Intn(7) - 3, IsInError: random.Intn(2) == 0,
		}
		b := RecoveryErrorStatus{
			Cost: uint32(random.Intn(3000)), NodeCount: random.Intn(20),
			DynPrec: random.Intn(7) - 3, IsInError: random.Intn(2) == 0,
		}
		forward := RecoveryCompareVersions(a, b)
		backward := RecoveryCompareVersions(b, a)
		if backward != mirror[forward] {
			t.Fatalf("trial %d: RecoveryCompareVersions(a,b)=%v but RecoveryCompareVersions(b,a)=%v, want %v",
				trial, forward, backward, mirror[forward])
		}
	}
}
