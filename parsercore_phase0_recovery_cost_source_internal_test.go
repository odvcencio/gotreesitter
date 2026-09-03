//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// recoveryCostSourceTable is an inert table: pricing a published subtree
// never dispatches, it only resolves records the caller already published.
type recoveryCostSourceTable struct{}

func (recoveryCostSourceTable) Actions(core.StateID, core.Symbol) (core.ActionRow, error) {
	return core.ActionRow{}, nil
}
func (recoveryCostSourceTable) Goto(core.StateID, core.Symbol) (core.StateID, error) { return 0, nil }
func (recoveryCostSourceTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}
func (recoveryCostSourceTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func newRecoveryCostFixture(t *testing.T, source string) (*core.Core, *diagnosticParserCoreRecoveryCostSource) {
	t.Helper()
	compact, err := core.New(recoveryCostSourceTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	src, err := newDiagnosticParserCoreRecoveryCostSource(compact, []byte(source))
	if err != nil {
		t.Fatalf("new cost source: %v", err)
	}
	return compact, src
}

// visibleSymbols marks every symbol below the given ceiling visible, which is
// what RecoverySymbolVisible reads when pricing an ERROR node's children.
func visibleSymbols(count int) []core.SelectedSymbolPolicy {
	out := make([]core.SelectedSymbolPolicy, count)
	for i := range out {
		out[i] = core.SelectedSymbolPolicy{Visible: true, Named: true}
	}
	return out
}

// TestRecoveryCostSourcePricesMissingLeaf pins C's missing-subtree cost.
// ts_subtree_error_cost short-circuits on the missing bit and returns
// ERROR_COST_PER_MISSING_TREE + ERROR_COST_PER_RECOVERY (subtree.h:331-337),
// which is 610 -- NOT ERROR_COST_PER_MISSING_TREE alone. Getting this wrong
// inverts every arbitration against an absorbing version, whose floor is 500.
func TestRecoveryCostSourcePricesMissingLeaf(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abc")
	id, err := compact.MissingLeaf(core.Symbol(3), 1)
	if err != nil {
		t.Fatalf("MissingLeaf: %v", err)
	}
	cost, err := core.RecoveryNodeErrorCost(visibleSymbols(8), src, id)
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	const want = core.RecoveryCostPerMissingTree + core.RecoveryCostPerRecovery
	if cost != want {
		t.Fatalf("missing leaf cost = %d, want %d", cost, want)
	}
	if want != 610 {
		t.Fatalf("the C constants no longer sum to 610 (got %d); re-derive every arbitration that depends on it", want)
	}
}

// TestRecoveryCostSourcePricesCleanTerminalZero proves an ordinary published
// terminal costs nothing, so pricing never charges a clean lineage.
func TestRecoveryCostSourcePricesCleanTerminalZero(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abc")
	id, err := compact.ErrorRegionLeaf(core.Symbol(4), 0, 3, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	cost, err := core.RecoveryNodeErrorCost(visibleSymbols(8), src, id)
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	if cost != 0 {
		t.Fatalf("clean terminal cost = %d, want 0", cost)
	}
}

// TestRecoveryCostSourcePricesErrorRegionSpanAndRows pins the ERROR-node
// formula: ERROR_COST_PER_RECOVERY, plus one per skipped byte, plus thirty per
// skipped row, plus ERROR_COST_PER_SKIPPED_TREE per visible child
// (subtree.c:400-402,442-444). The row term is why this source computes rows
// from the source text at all.
func TestRecoveryCostSourcePricesErrorRegionSpanAndRows(t *testing.T) {
	// Two rows spanned: the region covers bytes 0..7 of "ab\ncd\nef".
	compact, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	child, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 7, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	head, err := compact.ErrorRegionResume(seed, core.StateID(1), 0, 7, []core.SubtreeID{child})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	derivations, err := compact.Derivations(head)
	if err != nil || len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("expected one derivation carrying the ERROR container, got %v (err %v)", derivations, err)
	}
	cost, err := core.RecoveryNodeErrorCost(visibleSymbols(8), src, derivations[0].Payloads[0])
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	// span 0..7 = 7 bytes, rows 0..2 = 2 rows, one visible child.
	const want = core.RecoveryCostPerRecovery +
		core.RecoveryCostPerSkippedChar*7 +
		core.RecoveryCostPerSkippedLine*2 +
		core.RecoveryCostPerSkippedTree
	if cost != want {
		t.Fatalf("error region cost = %d, want %d", cost, want)
	}
	// This fixture spans two rows, so it prices ABOVE a missing insertion
	// (667 against 610). That direction is itself worth pinning: the row term
	// is what pushes a multi-line absorb past an insertion, and dropping it
	// would silently flip this fixture to the other side.
	if want <= core.RecoveryCostPerMissingTree+core.RecoveryCostPerRecovery {
		t.Fatalf("multi-row absorb priced at %d, expected above a missing insertion's %d",
			want, core.RecoveryCostPerMissingTree+core.RecoveryCostPerRecovery)
	}
}

// TestRecoveryCostSourcePublishesResumedHeadCost proves that the authenticated
// ERROR cost reaches the new head before its boundary is published.
func TestRecoveryCostSourcePublishesResumedHeadCost(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	child, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 7, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var memo core.RecoveryCostMemo
	cost := func(prev core.NodeID, payload core.SubtreeID) (uint32, error) {
		prefix, prefixErr := compact.RecoveryStoredErrorCost(core.Head{Node: prev})
		if prefixErr != nil {
			return 0, prefixErr
		}
		payloadCost, payloadErr := core.RecoveryNodeErrorCostMemo(
			visibleSymbols(8), src, &memo, payload,
		)
		if payloadErr != nil {
			return 0, payloadErr
		}
		return prefix + payloadCost, nil
	}
	head, err := compact.ErrorRegionResumeWithCost(
		seed, core.StateID(1), 0, 7, []core.SubtreeID{child}, cost,
	)
	if err != nil {
		t.Fatalf("ErrorRegionResumeWithCost: %v", err)
	}
	got, err := compact.RecoveryStoredErrorCost(head)
	if err != nil {
		t.Fatal(err)
	}
	const want = core.RecoveryCostPerRecovery +
		core.RecoveryCostPerSkippedChar*7 +
		core.RecoveryCostPerSkippedLine*2 +
		core.RecoveryCostPerSkippedTree
	if got != want {
		t.Fatalf("resumed head stored cost=%d, want %d", got, want)
	}
	if got != 667 {
		t.Fatalf("resumed head stored cost=%d, want locked value 667", got)
	}
}

// TestRecoveryCostSourceRowsTrackNewlines pins the row computation the ERROR
// formula depends on.
func TestRecoveryCostSourceRowsTrackNewlines(t *testing.T) {
	_, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	for _, tc := range []struct {
		offset uint32
		want   uint32
	}{{0, 0}, {2, 0}, {3, 1}, {5, 1}, {6, 2}, {8, 2}} {
		if got := src.rowAt(tc.offset); got != tc.want {
			t.Fatalf("rowAt(%d) = %d, want %d", tc.offset, got, tc.want)
		}
	}
	noNewlines := &diagnosticParserCoreRecoveryCostSource{source: []byte("abc")}
	if got := noNewlines.rowAt(3); got != 0 {
		t.Fatalf("rowAt on a single-line source = %d, want 0", got)
	}
}

// TestRecoveryLineageErrorCostSumsPayloads proves lineage pricing is C's
// ts_stack_error_cost: the SUM over the version's stack nodes, not the cost of
// the last one. The fixture puts two ERROR regions on one lineage and requires
// the total to be both regions added together.
func TestRecoveryLineageErrorCostSumsPayloads(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	firstChild, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 2, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	head, err := compact.ErrorRegionResume(seed, core.StateID(1), 0, 2, []core.SubtreeID{firstChild})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	secondChild, err := compact.ErrorRegionLeaf(core.Symbol(6), 2, 7, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	head, err = compact.ErrorRegionResume(head, core.StateID(2), 2, 7, []core.SubtreeID{secondChild})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}

	cost, err := diagnosticParserCoreLineageErrorCost(compact, head, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("lineage cost: %v", err)
	}
	// First region: span 0..2 = 2 bytes, 0 rows, one visible child.
	// Second region: span 2..7 = 5 bytes, 2 rows, one visible child.
	const firstRegion = core.RecoveryCostPerRecovery +
		core.RecoveryCostPerSkippedChar*2 + core.RecoveryCostPerSkippedTree
	const secondRegion = core.RecoveryCostPerRecovery +
		core.RecoveryCostPerSkippedChar*5 + core.RecoveryCostPerSkippedLine*2 +
		core.RecoveryCostPerSkippedTree
	if cost != firstRegion+secondRegion {
		t.Fatalf("lineage cost = %d, want %d (%d + %d): pricing did not sum the payloads",
			cost, firstRegion+secondRegion, firstRegion, secondRegion)
	}
}

// TestRecoveryLineageErrorCostMemoAgrees proves the memoized and unmemoized
// walks agree, since arbitration will run the memoized one.
func TestRecoveryLineageErrorCostMemoAgrees(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	child, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 7, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	head, err := compact.ErrorRegionResume(seed, core.StateID(1), 0, 7, []core.SubtreeID{child})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	plain, err := diagnosticParserCoreLineageErrorCost(compact, head, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("plain: %v", err)
	}
	var memo core.RecoveryCostMemo
	memoized, err := diagnosticParserCoreLineageErrorCost(compact, head, visibleSymbols(8), src, &memo)
	if err != nil {
		t.Fatalf("memoized: %v", err)
	}
	if plain != memoized {
		t.Fatalf("memoized cost %d disagrees with unmemoized %d", memoized, plain)
	}
	if plain == 0 {
		t.Fatal("fixture priced at zero; it proves nothing about agreement")
	}
}

// TestRecoveryLineageErrorCostDeclinesAmbiguousHead proves an ambiguous head
// is refused rather than priced along one arbitrary path. Every arbitration
// this supports is defined on a single lineage, and silently pricing one of
// several paths would make the comparison meaningless in exactly the case
// where it matters most.
func TestRecoveryLineageErrorCostDeclinesAmbiguousHead(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abcdef")
	seedA, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	seedB, err := compact.Seed(core.StateID(2), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	childA, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 3, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	childB, err := compact.ErrorRegionLeaf(core.Symbol(6), 0, 3, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	if _, err := compact.ErrorRegionResume(seedA, core.StateID(9), 0, 3, []core.SubtreeID{childA}); err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	// A second predecessor attaching at the same (state, byte offset) boundary
	// leaves the resulting head carrying two root-to-head paths.
	ambiguous, err := compact.ErrorRegionResume(seedB, core.StateID(9), 0, 3, []core.SubtreeID{childB})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	derivations, err := compact.Derivations(ambiguous)
	if err != nil {
		t.Fatalf("Derivations: %v", err)
	}
	if len(derivations) < 2 {
		t.Fatalf("fixture did not build an ambiguous head: %d derivations", len(derivations))
	}
	if _, err := diagnosticParserCoreLineageErrorCost(compact, ambiguous, visibleSymbols(8), src, nil); !errors.Is(err, diagnosticParserCoreLineageCostUnavailable) {
		t.Fatalf("ambiguous head priced without refusing: %v", err)
	}
}

// The selection ladder is the compact port of ts_parser__select_tree
// (parser.c:836-878). Each test below pins one rung, in C's own order.

func lineage(cost uint32, score int64) diagnosticParserCoreLineage {
	return diagnosticParserCoreLineage{Cost: cost, Score: score}
}

// TestSelectRecoveryLineagePrefersLowerCost pins rung 1, the rung the whole
// arbitration exists for: a missing insertion costs a flat 610 and an absorbed
// span costs 500 plus its length, so the cheaper tree is the one C publishes.
func TestSelectRecoveryLineagePrefersLowerCost(t *testing.T) {
	// 609 is the measured cost of absorbing the nine bytes of php "namespace"
	// (500 recovery + 9 chars + 100 for one visible child); 610 is a single
	// missing insertion. C publishes the ERROR tree, and so must this.
	winner, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(610, 0), lineage(609, 0),
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 1 {
		t.Fatalf("winner=%d, want 1 (the 609 absorb beats the 610 insertion)", winner)
	}
	// And the same pair in the opposite order must give the same answer.
	winner, err = diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(609, 0), lineage(610, 0),
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 0 {
		t.Fatalf("winner=%d, want 0; the ladder is order-sensitive on cost", winner)
	}
}

// TestSelectRecoveryLineagePrefersHigherPrecedenceOnCostTie pins rung 2.
func TestSelectRecoveryLineagePrefersHigherPrecedenceOnCostTie(t *testing.T) {
	winner, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(700, 3), lineage(700, 9),
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 1 {
		t.Fatalf("winner=%d, want 1 (higher dynamic precedence on a cost tie)", winner)
	}
	winner, err = diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(700, 9), lineage(700, 3),
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 0 {
		t.Fatalf("winner=%d, want 0", winner)
	}
}

// TestSelectRecoveryLineageTakesTheLaterErroredCandidateOnFullTie pins rung 3,
// the rung easiest to get backwards. C returns TRUE at parser.c:864 -- when
// cost and precedence both tie and the incumbent carries error content, the
// LATER candidate replaces it. Keeping the incumbent instead would look like a
// harmless stability choice and would silently diverge from C.
func TestSelectRecoveryLineageTakesTheLaterErroredCandidateOnFullTie(t *testing.T) {
	winner, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(610, 0), lineage(610, 0), lineage(610, 0),
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 2 {
		t.Fatalf("winner=%d, want 2: C takes the later candidate when both sides are errored and tied", winner)
	}
}

// TestSelectRecoveryLineageRefusesACleanTie pins rung 4 as OUT OF SCOPE. C
// falls back to a structural tree comparison, which this port does not model,
// and it can only be reached when both candidates are clean -- which no
// recovery lineage is. Declining beats guessing.
func TestSelectRecoveryLineageRefusesACleanTie(t *testing.T) {
	_, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(0, 0), lineage(0, 0),
	})
	if !errors.Is(err, errDiagnosticParserCoreLineageTie) {
		t.Fatalf("clean tie returned %v, want errDiagnosticParserCoreLineageTie", err)
	}
}

// TestSelectRecoveryLineageSingletonAndEmpty covers the degenerate inputs.
func TestSelectRecoveryLineageSingletonAndEmpty(t *testing.T) {
	winner, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{lineage(610, 0)})
	if err != nil || winner != 0 {
		t.Fatalf("singleton select = (%d, %v), want (0, nil)", winner, err)
	}
	if _, err := diagnosticParserCoreSelectRecoveryLineage(nil); err == nil {
		t.Fatal("selecting among no lineages returned no error")
	}
}

// TestPriceLineagesRefusesAmbiguousHead proves pricing carries the
// single-lineage requirement through to the selection entry point, rather than
// silently pricing one arbitrary path of an ambiguous head.
func TestPriceLineagesRefusesAmbiguousHead(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abcdef")
	seedA, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	seedB, err := compact.Seed(core.StateID(2), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	childA, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 3, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	childB, err := compact.ErrorRegionLeaf(core.Symbol(6), 0, 3, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	if _, err := compact.ErrorRegionResume(seedA, core.StateID(9), 0, 3, []core.SubtreeID{childA}); err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	ambiguous, err := compact.ErrorRegionResume(seedB, core.StateID(9), 0, 3, []core.SubtreeID{childB})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	if _, err := diagnosticParserCorePriceLineages([]diagnosticParserCoreLineageInput{{Head: ambiguous}}, visibleSymbols(8), src, nil); !errors.Is(err, diagnosticParserCoreLineageCostUnavailable) {
		t.Fatalf("pricing an ambiguous head returned %v, want a refusal", err)
	}
}

// TestPriceLineagesPricesEachHeadIndependently proves pricing walks each head
// rather than reusing one answer, and that the prices it produces are the ones
// the ladder then orders.
func TestPriceLineagesPricesEachHeadIndependently(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "ab\ncd\nef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	narrowChild, err := compact.ErrorRegionLeaf(core.Symbol(5), 0, 2, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	narrow, err := compact.ErrorRegionResume(seed, core.StateID(3), 0, 2, []core.SubtreeID{narrowChild})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	wideChild, err := compact.ErrorRegionLeaf(core.Symbol(6), 0, 7, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	wide, err := compact.ErrorRegionResume(seed, core.StateID(4), 0, 7, []core.SubtreeID{wideChild})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}

	priced, err := diagnosticParserCorePriceLineages([]diagnosticParserCoreLineageInput{{Head: wide}, {Head: narrow}}, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(priced) != 2 {
		t.Fatalf("priced %d lineages, want 2", len(priced))
	}
	if priced[0].Cost <= priced[1].Cost {
		t.Fatalf("the wider absorbed span priced at %d, not above the narrower %d", priced[0].Cost, priced[1].Cost)
	}
	winner, err := diagnosticParserCoreSelectRecoveryLineage(priced)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 1 {
		t.Fatalf("winner=%d, want 1: the cheaper, narrower region", winner)
	}
}

// TestSelectArbitratesMissingInsertionAgainstErrorAbsorb is the end-to-end
// claim this whole module exists to make correct, driven through real heads
// rather than hand-written costs.
//
// It reconstructs the measured php witness "<?php namespace ; ?>": one lineage
// inserts a single MISSING leaf, the other absorbs the nine bytes of
// "namespace" into an ERROR region with one visible child. C publishes the
// ERROR tree, because 500 + 9 + 100 = 609 beats a missing insertion's flat
// 610. The margin is one point, so this exercises every term of the model at
// once -- a dropped skipped-tree charge, a dropped row term, or a missing
// short circuit on the missing bit all flip the answer.
func TestSelectArbitratesMissingInsertionAgainstErrorAbsorb(t *testing.T) {
	const source = "<?php namespace ; ?>"
	compact, src := newRecoveryCostFixture(t, source)
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Lineage A: insert one zero-width MISSING leaf.
	missingHead, err := compact.ShiftMissingLeaf(seed, core.StateID(2), core.Symbol(3), 16)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}

	// Lineage B: absorb "namespace" (bytes 6..15, nine characters, one
	// visible child, no newline) into an ERROR region.
	absorbed, err := compact.ErrorRegionLeaf(core.Symbol(4), 6, 15, false)
	if err != nil {
		t.Fatalf("ErrorRegionLeaf: %v", err)
	}
	absorbHead, err := compact.ErrorRegionResume(seed, core.StateID(3), 6, 15, []core.SubtreeID{absorbed})
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}

	priced, err := diagnosticParserCorePriceLineages([]diagnosticParserCoreLineageInput{
		{Head: missingHead}, {Head: absorbHead},
	}, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("price: %v", err)
	}

	const wantMissing = core.RecoveryCostPerMissingTree + core.RecoveryCostPerRecovery // 610
	const wantAbsorb = core.RecoveryCostPerRecovery +
		core.RecoveryCostPerSkippedChar*9 + core.RecoveryCostPerSkippedTree // 609
	if priced[0].Cost != wantMissing {
		t.Fatalf("missing lineage priced %d, want %d", priced[0].Cost, wantMissing)
	}
	if priced[1].Cost != wantAbsorb {
		t.Fatalf("absorb lineage priced %d, want %d", priced[1].Cost, wantAbsorb)
	}
	if wantAbsorb >= wantMissing {
		t.Fatalf("fixture lost the one-point margin: absorb %d, missing %d", wantAbsorb, wantMissing)
	}

	winner, err := diagnosticParserCoreSelectRecoveryLineage(priced)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if winner != 1 {
		t.Fatalf("winner=%d, want 1: C publishes the ERROR tree for this witness", winner)
	}
}

// TestPriceLineagesChargesOpenRecoverySegments pins C's non-subtree stack
// cost, cStackOpenRecoveryCost (parser_recover_c.go:2475-2489). A head paused
// with no open error node yet carries 500 that no published subtree accounts
// for, and each extra error_repeat segment adds another 500.
func TestPriceLineagesChargesOpenRecoverySegments(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abcdef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	head, err := compact.ShiftMissingLeaf(seed, core.StateID(2), core.Symbol(3), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}

	priced, err := diagnosticParserCorePriceLineages([]diagnosticParserCoreLineageInput{
		{Head: head, OpenRecoverySegments: 0},
		{Head: head, OpenRecoverySegments: 1},
		{Head: head, OpenRecoverySegments: 3},
	}, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	base := priced[0].Cost
	if priced[1].Cost != base+core.RecoveryCostPerRecovery {
		t.Fatalf("one open-recovery segment added %d, want %d",
			priced[1].Cost-base, core.RecoveryCostPerRecovery)
	}
	if priced[2].Cost != base+3*core.RecoveryCostPerRecovery {
		t.Fatalf("three open-recovery segments added %d, want %d",
			priced[2].Cost-base, 3*core.RecoveryCostPerRecovery)
	}
	// The term is large enough to invert the arbitration on its own: an
	// otherwise-cheaper absorb loses once it is charged for a paused segment.
	if base+core.RecoveryCostPerRecovery <= base {
		t.Fatal("open-recovery term did not raise the price at all")
	}
}

// TestSelectRecoveryLineageResolvesWhenALaterCandidateDominates proves the
// fold declines only when the FINAL winner is undetermined, not whenever some
// pair along the way is.
//
// C folds pairwise too, and a later candidate can dominate both sides of an
// earlier unresolvable tie. Aborting at the pair would refuse an input C
// decides unambiguously.
func TestSelectRecoveryLineageResolvesWhenALaterCandidateDominates(t *testing.T) {
	// Two clean, tied lineages that this port cannot order against each
	// other, followed by one that beats both on cost.
	winner, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(0, 0), lineage(0, 0), lineage(0, 5),
	})
	if err != nil {
		t.Fatalf("select declined an input with a dominating candidate: %v", err)
	}
	if winner != 2 {
		t.Fatalf("winner=%d, want 2 (higher precedence beats both tied lineages)", winner)
	}

	// But an unresolvable tie involving the eventual winner still declines.
	if _, err := diagnosticParserCoreSelectRecoveryLineage([]diagnosticParserCoreLineage{
		lineage(0, 5), lineage(0, 5),
	}); !errors.Is(err, errDiagnosticParserCoreLineageTie) {
		t.Fatalf("a tie with the winner returned %v, want errDiagnosticParserCoreLineageTie", err)
	}
}
