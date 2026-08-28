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
	return compact, &diagnosticParserCoreRecoveryCostSource{compact: compact, source: []byte(source)}
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
	// The measured php arbitration turns on exactly this comparison: an
	// absorbed span must beat a missing insertion's flat 610 to win.
	if want >= core.RecoveryCostPerMissingTree+core.RecoveryCostPerRecovery {
		t.Logf("note: this fixture's absorb cost %d does not beat a missing insertion (610)", want)
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
// ts_stack_error_cost: the sum over the version's stack nodes.
func TestRecoveryLineageErrorCostSumsPayloads(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abcdef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	head, err := compact.ShiftMissingLeaf(seed, core.StateID(2), core.Symbol(3), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}
	head, err = compact.ShiftMissingLeaf(head, core.StateID(3), core.Symbol(4), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}
	cost, err := diagnosticParserCoreLineageErrorCost(compact, head, visibleSymbols(8), src, nil)
	if err != nil {
		t.Fatalf("lineage cost: %v", err)
	}
	const want = 2 * (core.RecoveryCostPerMissingTree + core.RecoveryCostPerRecovery)
	if cost != want {
		t.Fatalf("lineage cost = %d, want %d (two missing insertions)", cost, want)
	}
}

// TestRecoveryLineageErrorCostMemoAgrees proves the memoized and unmemoized
// walks agree, since arbitration will run the memoized one.
func TestRecoveryLineageErrorCostMemoAgrees(t *testing.T) {
	compact, src := newRecoveryCostFixture(t, "abcdef")
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	head, err := compact.ShiftMissingLeaf(seed, core.StateID(2), core.Symbol(3), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
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
	if _, err := compact.ShiftMissingLeaf(seedA, core.StateID(9), core.Symbol(3), 0); err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
	}
	// Two distinct predecessors attaching at the same (state, byte offset)
	// boundary leave the resulting head carrying two root-to-head paths.
	ambiguous, err := compact.ShiftMissingLeaf(seedB, core.StateID(9), core.Symbol(4), 0)
	if err != nil {
		t.Fatalf("ShiftMissingLeaf: %v", err)
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
