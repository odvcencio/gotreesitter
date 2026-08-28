package parsercorephase0

import "testing"

// electionChainFixture builds a four-node single-path chain
// H0(state 10) --sym5--> H1(state 11) --sym5--> H2(state 12) --sym5--> H3(state 13)
// over a fake table, returning H3 (the head every test elects from) plus the
// three terminal SubtreeIDs published along the way, oldest first
// (H0->H1, H1->H2, H2->H3).
func electionChainFixture(t *testing.T, tables *fakeTable) (compact *Core, head Head, payloads [3]SubtreeID) {
	t.Helper()
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	h0, err := compact.Seed(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	step := func(from Head, ordinal int, endByte uint32) Head {
		shifted, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{
			{Head: from, ActionOrdinal: ordinal},
		}, 5, Token{Symbol: 5, StartByte: endByte - 1, EndByte: endByte})
		if err != nil {
			t.Fatal(err)
		}
		if len(shifted) != 1 {
			t.Fatalf("shifted length=%d, want 1", len(shifted))
		}
		return shifted[0]
	}
	h1 := step(h0, 0, 1)
	h2 := step(h1, 0, 2)
	h3 := step(h2, 0, 3)
	// Derivations returns the FULL path of payloads back to the seed
	// (oldest first), not just the last hop's own terminal -- so head i
	// (0-indexed: h1, h2, h3) carries i+1 payloads, and the one this hop
	// itself just attached is the last entry.
	for i, h := range []Head{h1, h2, h3} {
		paths, err := compact.Derivations(h)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || len(paths[0].Payloads) != i+1 {
			t.Fatalf("head %d paths=%+v, want one derivation with %d payload(s)", i, paths, i+1)
		}
		payloads[i] = paths[0].Payloads[len(paths[0].Payloads)-1]
	}
	return compact, h3, payloads
}

func TestElectRecoveryTargetFindsShallowestAcceptingAncestor(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 10, symbol: 5}: {{Type: ActionShift, State: 11}},
		{state: 11, symbol: 5}: {{Type: ActionShift, State: 12}},
		{state: 12, symbol: 5}: {{Type: ActionShift, State: 13}},
		// Depth 1 ancestor (state 12) has no action for lookahead 7.
		// Depth 2 ancestor (state 11) does: this is the elected target.
		{state: 11, symbol: 7}: {{Type: ActionShift, State: 99}},
	}}
	compact, head, payloads := electionChainFixture(t, tables)

	target, ok, err := compact.ElectRecoveryTarget(head, 7, cRecoverMaxSummaryDepthForTest)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ElectRecoveryTarget ok=false, want a depth-2 election")
	}
	if target.Depth != 2 {
		t.Fatalf("target.Depth=%d, want 2", target.Depth)
	}
	if target.State != 11 {
		t.Fatalf("target.State=%d, want 11", target.State)
	}
	if target.ByteOffset != 1 {
		t.Fatalf("target.ByteOffset=%d, want 1 (H1's own boundary)", target.ByteOffset)
	}
	if len(target.Popped) != 2 {
		t.Fatalf("len(target.Popped)=%d, want 2", len(target.Popped))
	}
	// Nearest-head-first: Popped[0] is the H2->H3 payload, Popped[1] is the
	// H1->H2 payload (the one attached immediately above the elected
	// ancestor H1).
	if target.Popped[0] != payloads[2] || target.Popped[1] != payloads[1] {
		t.Fatalf("target.Popped=%v, want [%d, %d] (nearest-head-first)", target.Popped, payloads[2], payloads[1])
	}
}

func TestElectRecoveryTargetDeclinesWhenNoAncestorAccepts(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 10, symbol: 5}: {{Type: ActionShift, State: 11}},
		{state: 11, symbol: 5}: {{Type: ActionShift, State: 12}},
		{state: 12, symbol: 5}: {{Type: ActionShift, State: 13}},
		// No state anywhere in the chain has an action for lookahead 7.
	}}
	compact, head, _ := electionChainFixture(t, tables)

	_, ok, err := compact.ElectRecoveryTarget(head, 7, cRecoverMaxSummaryDepthForTest)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ElectRecoveryTarget ok=true, want false: no ancestor accepts lookahead 7")
	}
}

func TestElectRecoveryTargetDeclinesAtStackBase(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 10, symbol: 5}: {{Type: ActionShift, State: 11}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	h0, err := compact.Seed(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{
		{Head: h0, ActionOrdinal: 0},
	}, 5, Token{Symbol: 5, StartByte: 0, EndByte: 1})
	if err != nil {
		t.Fatal(err)
	}
	// h0 was itself Seed()-ed: it carries no incoming link at all (the
	// stack base). Walking past it (maxDepth=2 from a chain only one hop
	// deep) must decline, not panic or fabricate a target.
	_, ok, err := compact.ElectRecoveryTarget(shifted[0], 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ElectRecoveryTarget ok=true, want false: the walk reaches the stack base with no accepting ancestor")
	}
}

func TestElectRecoveryTargetDeclinesOnMultiPathBranch(t *testing.T) {
	// Two independent seeds shift into the SAME target state on the same
	// terminal: the resulting node carries two incoming links, a genuine
	// multi-path branch (mirrors TestShiftOrdinaryCohortSharesOneTerminalPayload).
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 1, symbol: 9}: {{Type: ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: ActionShift, State: 3}},
		// If the branch guard were absent, this action would make depth 1
		// electable -- proving the decline below is the branch guard, not
		// an unrelated "no accepting ancestor" outcome (mutation coverage
		// for restriction 1's decline).
		{state: 1, symbol: 7}: {{Type: ActionShift, State: 99}},
		{state: 2, symbol: 7}: {{Type: ActionShift, State: 99}},
	}}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := compact.Seed(1, 4)
	second, _ := compact.Seed(2, 4)
	shifted, err := compact.ShiftOrdinaryCohort([]OrdinaryCohortShiftInput{
		{Head: first, ActionOrdinal: 0},
		{Head: second, ActionOrdinal: 0},
	}, 9, Token{Symbol: 9, StartByte: 4, EndByte: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != 2 {
		t.Fatalf("shifted length=%d, want 2", len(shifted))
	}
	merged := shifted[1] // the shared-target node, two incoming links

	_, ok, err := compact.ElectRecoveryTarget(merged, 7, 3)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ElectRecoveryTarget ok=true, want false: the merged node has two incoming links, a multi-path branch this sub-unit declines")
	}
}

func TestElectRecoveryTargetRejectsNonPositiveMaxDepth(t *testing.T) {
	tables := &fakeTable{actions: map[tableCell][]Action{
		{state: 10, symbol: 5}: {{Type: ActionShift, State: 11}},
		{state: 11, symbol: 5}: {{Type: ActionShift, State: 12}},
		{state: 12, symbol: 5}: {{Type: ActionShift, State: 13}},
	}}
	compact, head, _ := electionChainFixture(t, tables)
	if _, ok, err := compact.ElectRecoveryTarget(head, 7, 0); err != nil || ok {
		t.Fatalf("ElectRecoveryTarget(maxDepth=0) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	if _, ok, err := compact.ElectRecoveryTarget(head, 7, -1); err != nil || ok {
		t.Fatalf("ElectRecoveryTarget(maxDepth=-1) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

// cRecoverMaxSummaryDepthForTest mirrors the production
// cRecoverMaxSummaryDepth (parser_recover_c.go, value 16) without importing
// the root package: this test file only needs a bound comfortably larger
// than its own three-hop fixture chain.
const cRecoverMaxSummaryDepthForTest = 16
