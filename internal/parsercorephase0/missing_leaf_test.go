package parsercorephase0

import "testing"

// MissingLeaf is the compact representation of C's
// ts_subtree_new_missing_leaf (subtree.c:534-546). These tests drive the Core
// API directly.

func newMissingLeafTestCore(t *testing.T) *Core {
	t.Helper()
	compact, err := New(&fakeTable{}, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

// TestMissingLeafPublishesZeroWidthTerminal pins every field C's construction
// fixes: zero width at the stack position, terminal, missing, and neither
// extra nor external.
func TestMissingLeafPublishesZeroWidthTerminal(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	const symbol = Symbol(7)
	const atByte = uint32(41)

	id, err := compact.MissingLeaf(symbol, atByte)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	view, err := compact.Subtree(id)
	if err != nil {
		t.Fatalf("Subtree: %v", err)
	}
	if view.Symbol != symbol {
		t.Fatalf("symbol = %d, want %d", view.Symbol, symbol)
	}
	if view.StartByte != atByte || view.EndByte != atByte {
		t.Fatalf("span = [%d,%d), want the zero-width span [%d,%d)",
			view.StartByte, view.EndByte, atByte, atByte)
	}
	if !view.Terminal {
		t.Fatal("a missing leaf must be a terminal record")
	}
	if !view.Missing {
		t.Fatal("a missing leaf must carry the missing bit")
	}
	// C passes false for external tokens and never marks the leaf extra.
	// Marking it extra would additionally make popPaths skip it when counting
	// a production's structural arity, silently changing every enclosing
	// reduce.
	if view.Extra {
		t.Fatal("a missing leaf must not be extra")
	}
	if view.External {
		t.Fatal("a missing leaf must not be external")
	}
	if len(view.Children) != 0 {
		t.Fatalf("a missing leaf must have no children, got %d", len(view.Children))
	}
}

// TestMissingLeafRejectsReservedSymbols proves the two RESERVED symbols that
// can never name a missing token fail closed. It deliberately does not claim
// more: MissingLeaf takes no StateID, so it cannot check that the grammar
// actually demands the token, and an ordinary grammar nonterminal is accepted.
// See MissingLeaf's own doc comment for what the caller still owes.
func TestMissingLeafRejectsReservedSymbols(t *testing.T) {
	for name, symbol := range map[string]Symbol{
		"end-of-file": 0,
		"error":       ErrorRegionSymbol,
	} {
		t.Run(name, func(t *testing.T) {
			compact := newMissingLeafTestCore(t)
			if _, err := compact.MissingLeaf(symbol, 0); err == nil {
				t.Fatalf("MissingLeaf accepted the %s symbol", name)
			}
		})
	}
}

// TestMissingLeafSurfacesThroughMaterializationView proves the bit reaches the
// materializer, which is the consumer that turns it into the public node's own
// missing and has-error bits.
func TestMissingLeafSurfacesThroughMaterializationView(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	missingID, err := compact.MissingLeaf(Symbol(3), 12)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	ordinaryID, err := compact.ErrorRegionLeaf(Symbol(3), 12, 15, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}

	missingView, err := compact.MaterializationView(missingID)
	if err != nil {
		t.Fatalf("MaterializationView(missing): %v", err)
	}
	if !missingView.Missing {
		t.Fatal("materialization view lost the missing bit")
	}
	ordinaryView, err := compact.MaterializationView(ordinaryID)
	if err != nil {
		t.Fatalf("MaterializationView(ordinary): %v", err)
	}
	if ordinaryView.Missing {
		t.Fatal("an ordinary published terminal reported the missing bit")
	}
}

// TestMissingLeafIsExplicit checks the stage boundary over the publish paths
// reachable from a bare Core. Only an explicit MissingLeaf call sets the bit.
//
// The test does not reach scheduler shift paths or Reduce. Those paths need a
// table with real actions instead of this inert fixture.
func TestMissingLeafIsExplicit(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	seed, err := compact.Seed(StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := compact.appendDiagnosticPayload(seed, StateID(1),
		Token{Symbol: 5, StartByte: 0, EndByte: 2}, pathMeta{}); err != nil {
		t.Fatalf("appendDiagnosticPayload: %v", err)
	}
	regionChild, err := compact.ErrorRegionLeaf(Symbol(6), 2, 4, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	if _, err := compact.ErrorRegionResume(seed, StateID(1), 2, 4, []SubtreeID{regionChild}); err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	for id := SubtreeID(1); uint64(id) <= uint64(len(compact.subtrees)); id++ {
		view, err := compact.Subtree(id)
		if err != nil {
			t.Fatalf("Subtree(%d): %v", id, err)
		}
		if view.Missing {
			t.Fatalf("subtree %d reported the missing bit without a MissingLeaf call", id)
		}
	}
}

// TestMissingLeafSurfacesThroughPostorderVisitor covers the path the driver
// actually materializes through.
//
// TestMissingLeafSurfacesThroughMaterializationView above exercises the
// RANDOM-ACCESS accessor, but public-node construction runs from
// VisitMaterializationPostorder(WithScratch), which builds its view at a
// different site (materialization_postorder_scratch.go). Without this test,
// deleting the Missing field from that construction would leave every other
// test in this file green while silently disabling the feature on the only
// path that ships.
func TestMissingLeafSurfacesThroughPostorderVisitor(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	missingID, err := compact.MissingLeaf(Symbol(3), 12)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	ordinaryID, err := compact.ErrorRegionLeaf(Symbol(3), 12, 15, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}

	seen := map[SubtreeID]bool{}
	visited := 0
	err = compact.VisitMaterializationPostorder(
		[]SubtreeID{missingID, ordinaryID},
		func() error { return nil },
		func(id SubtreeID, view MaterializationSubtreeView) error {
			seen[id] = view.Missing
			visited++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("VisitMaterializationPostorder: %v", err)
	}
	if visited != 2 {
		t.Fatalf("visited %d subtrees, want 2", visited)
	}
	if !seen[missingID] {
		t.Fatal("the postorder visitor lost the missing bit on the missing leaf")
	}
	if seen[ordinaryID] {
		t.Fatal("the postorder visitor reported an ordinary terminal as missing")
	}
}

// TestMissingLeafIsNotStructurallyEqualToCleanPayload pins the fail-closed
// half of the duplicate-drop gate.
//
// subtreesStructurallyEqual authorizes DROPPING one payload as a duplicate of
// another. A recovery-inserted MISSING leaf and a clean zero-width payload
// with the same symbol and span agree on every other compared field, so
// without the missing bit in that predicate the MISSING record would be
// discarded and its error lost from the published tree.
func TestMissingLeafIsNotStructurallyEqualToCleanPayload(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	missingID, err := compact.MissingLeaf(Symbol(3), 12)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	// Same symbol, same zero-width span, published the ordinary way.
	cleanID, err := compact.ErrorRegionLeaf(Symbol(3), 12, 12, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	equal, err := compact.subtreesStructurallyEqual(missingID, cleanID)
	if err != nil {
		t.Fatalf("subtreesStructurallyEqual: %v", err)
	}
	if equal {
		t.Fatal("a MISSING payload compared structurally equal to a clean zero-width payload; the duplicate-drop gate would discard the error")
	}
	// Control: the predicate still folds two genuinely identical clean payloads.
	otherCleanID, err := compact.ErrorRegionLeaf(Symbol(3), 12, 12, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	equal, err = compact.subtreesStructurallyEqual(cleanID, otherCleanID)
	if err != nil {
		t.Fatalf("subtreesStructurallyEqual: %v", err)
	}
	if !equal {
		t.Fatal("two identical clean payloads stopped comparing equal; the missing bit over-separated them")
	}
}

// TestMissingLeafDropCohortReceiptDistinguishesTheBit proves the drop-cohort
// receipt digest and its total-order comparator both authenticate the bit.
// Both already tracked `fragile`, the other late-added record bit, so omitting
// `missing` would have been a missed site rather than a scoping decision: two
// receipts over different parses would hash identically.
func TestMissingLeafDropCohortReceiptDistinguishesTheBit(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	missingID, err := compact.MissingLeaf(Symbol(3), 12)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	cleanID, err := compact.ErrorRegionLeaf(Symbol(3), 12, 12, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	order, err := compact.dropCohortCompareSubtree(missingID, cleanID)
	if err != nil {
		t.Fatalf("dropCohortCompareSubtree: %v", err)
	}
	if order == 0 {
		t.Fatal("the drop-cohort comparator ordered a MISSING payload equal to a clean one")
	}
}

func TestUniqueStateSpineRequiresOneCompletePath(t *testing.T) {
	compact := newMissingLeafTestCore(t)
	seed, err := compact.Seed(StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	shifted, err := compact.ShiftMissingLeaf(seed, StateID(2), Symbol(7), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}
	spine, ok, err := compact.UniqueStateSpine(shifted, 2)
	if err != nil {
		t.Fatalf("UniqueStateSpine: %v", err)
	}
	if !ok || len(spine) != 2 || spine[0] != 1 || spine[1] != 2 {
		t.Fatalf("spine=%v ok=%t, want [1 2] true", spine, ok)
	}
	if _, ok, err := compact.UniqueStateSpine(shifted, 1); err != nil || ok {
		t.Fatalf("truncated spine returned ok=%t err=%v", ok, err)
	}

	first, err := compact.MissingLeaf(Symbol(8), 0)
	if err != nil {
		t.Fatalf("first payload: %v", err)
	}
	second, err := compact.MissingLeaf(Symbol(9), 0)
	if err != nil {
		t.Fatalf("second payload: %v", err)
	}
	key := compact.shiftedBoundaryKey(StateID(3), 0)
	ambiguous, err := compact.condense(key, linkInput{
		prev: seed.Node, payload: first,
		storedErrorCost: compactMissingLeafStoredErrorCost, hasStoredErrorCost: true,
	})
	if err != nil {
		t.Fatalf("first condense: %v", err)
	}
	ambiguous, err = compact.condense(key, linkInput{prev: shifted.Node, payload: second})
	if err != nil {
		t.Fatalf("second condense: %v", err)
	}
	if _, ok, err := compact.UniqueStateSpine(ambiguous, 8); err != nil || ok {
		t.Fatalf("ambiguous spine returned ok=%t err=%v", ok, err)
	}
}

// TestCanShiftAfterReductionsKeepsDistinctStackPrefixes pins a subtle
// simulation rule. Equal depth and top state do not identify a whole stack.
func TestCanShiftAfterReductionsKeepsDistinctStackPrefixes(t *testing.T) {
	const lookahead = Symbol(9)
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: lookahead}: {
				{Type: ActionReduce, Symbol: 10, ChildCount: 1},
				{Type: ActionReduce, Symbol: 11, ChildCount: 2},
			},
			{state: 6, symbol: lookahead}: {
				{Type: ActionReduce, Symbol: 12, ChildCount: 0},
			},
			{state: 4, symbol: lookahead}: {
				{Type: ActionReduce, Symbol: 13, ChildCount: 1},
			},
			{state: 8, symbol: lookahead}: {
				{Type: ActionShift, State: 9},
			},
		},
		gotos: map[tableCell]StateID{
			{state: 2, symbol: 10}: 4,
			{state: 1, symbol: 11}: 6,
			{state: 6, symbol: 12}: 4,
			{state: 2, symbol: 13}: 7,
			{state: 6, symbol: 13}: 8,
		},
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := compact.CanShiftAfterReductions([]StateID{1, 2, 3}, lookahead)
	if err != nil {
		t.Fatalf("CanShiftAfterReductions: %v", err)
	}
	if !got {
		t.Fatal("the viable reduction path was lost after two stacks reached the same depth and top state")
	}
}

func TestCanShiftAfterReductionsRejectsNonProgressShifts(t *testing.T) {
	const lookahead = Symbol(9)
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 2, symbol: lookahead}: {
			{Type: ActionShift, State: 2, Extra: true},
			{Type: ActionShift, State: 2, Repetition: true},
		},
	}}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := compact.CanShiftAfterReductions([]StateID{1, 2}, lookahead)
	if err != nil {
		t.Fatalf("CanShiftAfterReductions: %v", err)
	}
	if got {
		t.Fatal("an extra or repetition shift reported grammatical progress")
	}
}
