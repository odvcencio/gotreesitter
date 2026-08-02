package parsercorephase0

import (
	"errors"
	"math/rand"
	"testing"
)

// electionFixture builds one fake source shared by several derivations, so a
// test can publish distinct candidate trees and then compare them the way the
// election does.
type electionFixture struct {
	src  fakeRecoveryCostSource
	next SubtreeID
}

func newElectionFixture() *electionFixture {
	return &electionFixture{src: fakeRecoveryCostSource{}}
}

func (f *electionFixture) publish(spec *recoveryCostSpec) SubtreeID {
	return publishRecoveryCostSpec(f.src, &f.next, spec)
}

func leaf(symbol Symbol) *recoveryCostSpec {
	return &recoveryCostSpec{symbol: symbol}
}

func node(symbol Symbol, children ...*recoveryCostSpec) *recoveryCostSpec {
	return &recoveryCostSpec{symbol: symbol, children: children}
}

// TestAcceptanceElectionCompareSymbolRung pins ts_subtree_compare's first
// rung (subtree.c:600-601): the LOWER symbol id sorts first.
func TestAcceptanceElectionCompareSymbolRung(t *testing.T) {
	f := newElectionFixture()
	low := f.publish(leaf(3))
	high := f.publish(leaf(9))

	if got, err := AcceptanceElectionCompare(f.src, low, high); err != nil || got != -1 {
		t.Fatalf("compare(low, high) = %d, %v; want -1, nil", got, err)
	}
	if got, err := AcceptanceElectionCompare(f.src, high, low); err != nil || got != 1 {
		t.Fatalf("compare(high, low) = %d, %v; want 1, nil", got, err)
	}
	if got, err := AcceptanceElectionCompare(f.src, low, low); err != nil || got != 0 {
		t.Fatalf("compare(low, low) = %d, %v; want 0, nil", got, err)
	}
}

// TestAcceptanceElectionCompareChildCountRung pins ts_subtree_compare's
// second rung (subtree.c:602-603): with equal symbols, FEWER children sorts
// first.
func TestAcceptanceElectionCompareChildCountRung(t *testing.T) {
	f := newElectionFixture()
	fewer := f.publish(node(4, leaf(7)))
	more := f.publish(node(4, leaf(7), leaf(7)))

	if got, err := AcceptanceElectionCompare(f.src, fewer, more); err != nil || got != -1 {
		t.Fatalf("compare(fewer, more) = %d, %v; want -1, nil", got, err)
	}
	if got, err := AcceptanceElectionCompare(f.src, more, fewer); err != nil || got != 1 {
		t.Fatalf("compare(more, fewer) = %d, %v; want 1, nil", got, err)
	}
}

// TestAcceptanceElectionCompareRecursesLeftToRight pins ts_subtree_compare's
// third rung (subtree.c:609-614): with equal symbols and child counts, the
// children decide, and the FIRST differing child pair decides -- the C loop
// pushes children last-to-first onto a stack so the pairs pop left to right.
func TestAcceptanceElectionCompareRecursesLeftToRight(t *testing.T) {
	f := newElectionFixture()
	// Both roots have symbol 4 and two children. The first child pair ties;
	// the second differs. A right-to-left walk would report the same answer
	// here, so the third pair below makes the direction observable: the FIRST
	// difference must win even when a later pair disagrees with it.
	left := f.publish(node(4, leaf(7), leaf(2), leaf(9)))
	right := f.publish(node(4, leaf(7), leaf(5), leaf(1)))

	if got, err := AcceptanceElectionCompare(f.src, left, right); err != nil || got != -1 {
		t.Fatalf("compare(left, right) = %d, %v; want -1, nil (child 1: symbol 2 < 5)", got, err)
	}
	if got, err := AcceptanceElectionCompare(f.src, right, left); err != nil || got != 1 {
		t.Fatalf("compare(right, left) = %d, %v; want 1, nil", got, err)
	}
}

// TestAcceptanceElectionCompareIgnoresNonSymbolAxes pins what
// ts_subtree_compare deliberately does NOT read. Two subtrees with the same
// symbol, child count, and children compare equal even when their spans and
// extra flags differ, because C compares none of those. This is the exact
// property that makes the apex class_literal_alias family (two candidates
// differing only in a field assignment, which is a production-id property)
// resolve on the election's "keep the incumbent" answer rather than on a
// structural difference.
func TestAcceptanceElectionCompareIgnoresNonSymbolAxes(t *testing.T) {
	f := newElectionFixture()
	a := f.publish(&recoveryCostSpec{symbol: 4, start: 0, end: 10, children: []*recoveryCostSpec{leaf(7)}})
	b := f.publish(&recoveryCostSpec{symbol: 4, start: 3, end: 40, extra: true, children: []*recoveryCostSpec{leaf(7)}})

	if got, err := AcceptanceElectionCompare(f.src, a, b); err != nil || got != 0 {
		t.Fatalf("compare = %d, %v; want 0, nil", got, err)
	}
}

// TestAcceptanceElectionCompareRootsComparesCountThenElements pins the
// list-level comparison: a derivation is a list of accepted payload roots,
// compared the way ts_subtree_compare compares one node's children -- count
// first, then pairwise, left to right.
func TestAcceptanceElectionCompareRootsComparesCountThenElements(t *testing.T) {
	f := newElectionFixture()
	low := f.publish(leaf(3))
	high := f.publish(leaf(9))

	if got, err := AcceptanceElectionCompareRoots(f.src, []SubtreeID{low}, []SubtreeID{low, high}); err != nil || got != -1 {
		t.Fatalf("compare(1 root, 2 roots) = %d, %v; want -1, nil", got, err)
	}
	if got, err := AcceptanceElectionCompareRoots(f.src, []SubtreeID{low, high}, []SubtreeID{low}); err != nil || got != 1 {
		t.Fatalf("compare(2 roots, 1 root) = %d, %v; want 1, nil", got, err)
	}
	if got, err := AcceptanceElectionCompareRoots(f.src, []SubtreeID{high, low}, []SubtreeID{high, high}); err != nil || got != -1 {
		t.Fatalf("compare = %d, %v; want -1, nil (second root: 3 < 9)", got, err)
	}
	if got, err := AcceptanceElectionCompareRoots(f.src, []SubtreeID{low, high}, []SubtreeID{low, high}); err != nil || got != 0 {
		t.Fatalf("compare(equal, equal) = %d, %v; want 0, nil", got, err)
	}
}

// TestAcceptanceElectionCompareReportsSourceError pins that a malformed
// handle surfaces as an error instead of an invented order.
func TestAcceptanceElectionCompareReportsSourceError(t *testing.T) {
	f := newElectionFixture()
	present := f.publish(leaf(3))
	if _, err := AcceptanceElectionCompare(f.src, present, 999); !errors.Is(err, ErrRecoveryCostNodeMissing) {
		t.Fatalf("compare with an absent id: err = %v, want ErrRecoveryCostNodeMissing", err)
	}
	if _, err := AcceptanceElectionCompare(nil, present, present); err == nil {
		t.Fatal("compare with a nil source: err = nil, want an error")
	}
}

// TestAcceptanceElectionErrorCostSumsRoots pins that a derivation's error
// cost is the sum over its payload roots, the compact analogue of the root
// ts_parser__accept would have built.
func TestAcceptanceElectionErrorCostSumsRoots(t *testing.T) {
	f := newElectionFixture()
	clean := f.publish(node(ordinarySymbol, leaf(ordinarySymbol)))
	// An ERROR container spanning ten bytes and one row, wrapping one
	// visible skipped tree.
	erroneous := f.publish(&recoveryCostSpec{
		symbol: RecoveryErrorSymbol, start: 0, end: 10, endRow: 1,
		children: []*recoveryCostSpec{{symbol: ordinarySymbol}},
	})

	symbols := visibleSymbols()
	if got, err := AcceptanceElectionErrorCost(symbols, f.src, nil, []SubtreeID{clean}); err != nil || got != 0 {
		t.Fatalf("clean derivation cost = %d, %v; want 0, nil", got, err)
	}
	want := uint32(RecoveryCostPerSkippedTree + RecoveryCostPerRecovery + RecoveryCostPerSkippedChar*10 + RecoveryCostPerSkippedLine*1)
	if got, err := AcceptanceElectionErrorCost(symbols, f.src, nil, []SubtreeID{erroneous}); err != nil || got != want {
		t.Fatalf("erroneous derivation cost = %d, %v; want %d, nil", got, err, want)
	}
	if got, err := AcceptanceElectionErrorCost(symbols, f.src, nil, []SubtreeID{clean, erroneous}); err != nil || got != want {
		t.Fatalf("summed derivation cost = %d, %v; want %d, nil", got, err, want)
	}
	var memo RecoveryCostMemo
	if got, err := AcceptanceElectionErrorCost(symbols, f.src, &memo, []SubtreeID{clean, erroneous}); err != nil || got != want {
		t.Fatalf("memoized derivation cost = %d, %v; want %d, nil", got, err, want)
	}
}

// TestElectAcceptanceDerivationLadder walks every rung of
// ts_parser__select_tree in the order C evaluates them.
func TestElectAcceptanceDerivationLadder(t *testing.T) {
	f := newElectionFixture()
	symbols := visibleSymbols()
	cheapLow := f.publish(leaf(3))
	cheapHigh := f.publish(leaf(9))
	costly := f.publish(&recoveryCostSpec{
		symbol: RecoveryErrorSymbol, start: 0, end: 4,
		children: []*recoveryCostSpec{{symbol: ordinarySymbol}},
	})
	costlier := f.publish(&recoveryCostSpec{
		symbol: RecoveryErrorSymbol, start: 0, end: 40,
		children: []*recoveryCostSpec{{symbol: ordinarySymbol}},
	})

	tests := []struct {
		name     string
		paths    []Derivation
		wantIdx  int
		wantRung string
	}{
		{
			name:     "sole candidate",
			paths:    []Derivation{{Payloads: []SubtreeID{cheapHigh}}},
			wantIdx:  0,
			wantRung: AcceptanceElectionRungSoleCandidate,
		},
		{
			name: "rung 1: lower error cost takes the challenger",
			paths: []Derivation{
				{Payloads: []SubtreeID{costly}},
				{Payloads: []SubtreeID{cheapHigh}},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungErrorCost,
		},
		{
			name: "rung 2: lower error cost keeps the incumbent",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapHigh}},
				{Payloads: []SubtreeID{costly}},
			},
			wantIdx:  0,
			wantRung: AcceptanceElectionRungIncumbent,
		},
		{
			name: "rung 3: higher dynamic precedence takes the challenger",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapLow}, Score: 1},
				{Payloads: []SubtreeID{cheapHigh}, Score: 7},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungDynamicPrec,
		},
		{
			name: "rung 4: higher dynamic precedence keeps the incumbent",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapHigh}, Score: 7},
				{Payloads: []SubtreeID{cheapLow}, Score: 1},
			},
			wantIdx:  0,
			wantRung: AcceptanceElectionRungIncumbent,
		},
		{
			name: "rung 5: an equal nonzero error cost takes the later candidate",
			paths: []Derivation{
				{Payloads: []SubtreeID{costly}},
				{Payloads: []SubtreeID{costly}},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungErrorTakeLate,
		},
		{
			name: "rung 6: the comparator takes a lower-symbol challenger",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapHigh}},
				{Payloads: []SubtreeID{cheapLow}},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungComparator,
		},
		{
			name: "rung 6: the comparator keeps a lower-symbol incumbent",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapLow}},
				{Payloads: []SubtreeID{cheapHigh}},
			},
			wantIdx:  0,
			wantRung: AcceptanceElectionRungIncumbent,
		},
		{
			name: "rung 6 tie: C keeps the incumbent",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapLow}},
				{Payloads: []SubtreeID{cheapLow}},
			},
			wantIdx:  0,
			wantRung: AcceptanceElectionRungIncumbent,
		},
		{
			name: "error cost outranks dynamic precedence",
			paths: []Derivation{
				{Payloads: []SubtreeID{costlier}, Score: 99},
				{Payloads: []SubtreeID{costly}, Score: -99},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungErrorCost,
		},
		{
			name: "dynamic precedence outranks the comparator",
			paths: []Derivation{
				{Payloads: []SubtreeID{cheapLow}, Score: 1},
				{Payloads: []SubtreeID{cheapHigh}, Score: 2},
			},
			wantIdx:  1,
			wantRung: AcceptanceElectionRungDynamicPrec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, err := ElectAcceptanceDerivation(symbols, f.src, nil, tt.paths)
			if err != nil {
				t.Fatalf("elect: %v", err)
			}
			if outcome.Index != tt.wantIdx {
				t.Fatalf("elected index = %d, want %d (rung %q)", outcome.Index, tt.wantIdx, outcome.Rung)
			}
			if outcome.Rung != tt.wantRung {
				t.Fatalf("deciding rung = %q, want %q", outcome.Rung, tt.wantRung)
			}
		})
	}
}

// TestElectAcceptanceDerivationReportsCleanPathEvidence pins the outcome
// fields the certification sweeps read: a clean election reports zero
// MaxErrorCost, and the structural rung reports whether it decided or tied.
func TestElectAcceptanceDerivationReportsCleanPathEvidence(t *testing.T) {
	f := newElectionFixture()
	symbols := visibleSymbols()
	low := f.publish(leaf(3))
	high := f.publish(leaf(9))

	outcome, err := ElectAcceptanceDerivation(symbols, f.src, nil, []Derivation{
		{Payloads: []SubtreeID{high}},
		{Payloads: []SubtreeID{low}},
	})
	if err != nil {
		t.Fatalf("elect: %v", err)
	}
	if outcome.MaxErrorCost != 0 || outcome.ErrorCost != 0 {
		t.Fatalf("clean election costs = %d/%d, want 0/0", outcome.ErrorCost, outcome.MaxErrorCost)
	}
	if !outcome.ComparatorDecided || outcome.StructurallyTied {
		t.Fatalf("clean election evidence decided=%t tied=%t, want true/false",
			outcome.ComparatorDecided, outcome.StructurallyTied)
	}

	outcome, err = ElectAcceptanceDerivation(symbols, f.src, nil, []Derivation{
		{Payloads: []SubtreeID{low}},
		{Payloads: []SubtreeID{low}},
	})
	if err != nil {
		t.Fatalf("elect: %v", err)
	}
	if outcome.ComparatorDecided || !outcome.StructurallyTied {
		t.Fatalf("tied election evidence decided=%t tied=%t, want false/true",
			outcome.ComparatorDecided, outcome.StructurallyTied)
	}
}

// TestElectAcceptanceDerivationRejectsEmptyInput pins the two caller errors
// the election reports instead of guessing.
func TestElectAcceptanceDerivationRejectsEmptyInput(t *testing.T) {
	f := newElectionFixture()
	if _, err := ElectAcceptanceDerivation(nil, f.src, nil, nil); !errors.Is(err, ErrAcceptanceElectionEmpty) {
		t.Fatalf("empty candidate set: err = %v, want ErrAcceptanceElectionEmpty", err)
	}
	if _, err := ElectAcceptanceDerivation(nil, nil, nil, []Derivation{{}}); err == nil {
		t.Fatal("nil source: err = nil, want an error")
	}
}

// TestElectAcceptanceDerivationIsOrderStableUnderFold pins the property that
// makes the election deterministic: on a clean candidate set the fold elects
// the C-minimum under ts_subtree_compare, and it elects the SAME candidate
// no matter how many equal-comparing duplicates surround it, because C's tie
// answer keeps the incumbent.
func TestElectAcceptanceDerivationIsOrderStableUnderFold(t *testing.T) {
	f := newElectionFixture()
	symbols := visibleSymbols()
	ids := []SubtreeID{f.publish(leaf(11)), f.publish(leaf(4)), f.publish(leaf(7)), f.publish(leaf(4))}

	paths := make([]Derivation, 0, len(ids))
	for _, id := range ids {
		paths = append(paths, Derivation{Payloads: []SubtreeID{id}})
	}
	outcome, err := ElectAcceptanceDerivation(symbols, f.src, nil, paths)
	if err != nil {
		t.Fatalf("elect: %v", err)
	}
	// Index 1 and index 3 both carry symbol 4 and compare equal; C keeps the
	// earlier one.
	if outcome.Index != 1 {
		t.Fatalf("elected index = %d, want 1", outcome.Index)
	}
}

// TestElectAcceptanceDerivationMemoMatchesUnmemoized runs a model-based
// property check: the elected candidate never depends on whether the caller
// supplied an error-cost memo.
func TestElectAcceptanceDerivationMemoMatchesUnmemoized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x52322026))
	symbols := visibleSymbols()
	for trial := 0; trial < 200; trial++ {
		f := newElectionFixture()
		count := 2 + rng.Intn(5)
		paths := make([]Derivation, 0, count)
		for i := 0; i < count; i++ {
			roots := make([]SubtreeID, 0, 2)
			for r := 0; r < 1+rng.Intn(2); r++ {
				roots = append(roots, f.publish(randomElectionSpec(rng, 0)))
			}
			paths = append(paths, Derivation{Payloads: roots, Score: int64(rng.Intn(5)) - 2})
		}
		unmemoized, err := ElectAcceptanceDerivation(symbols, f.src, nil, paths)
		if err != nil {
			t.Fatalf("trial %d unmemoized: %v", trial, err)
		}
		var memo RecoveryCostMemo
		memoized, err := ElectAcceptanceDerivation(symbols, f.src, &memo, paths)
		if err != nil {
			t.Fatalf("trial %d memoized: %v", trial, err)
		}
		if unmemoized.Index != memoized.Index || unmemoized.Rung != memoized.Rung ||
			unmemoized.ErrorCost != memoized.ErrorCost {
			t.Fatalf("trial %d: unmemoized=%+v memoized=%+v", trial, unmemoized, memoized)
		}
	}
}

func randomElectionSpec(rng *rand.Rand, depth int) *recoveryCostSpec {
	spec := &recoveryCostSpec{symbol: Symbol(rng.Intn(4) + 4)}
	if rng.Intn(6) == 0 {
		spec.symbol = RecoveryErrorSymbol
		spec.start = 0
		spec.end = uint32(rng.Intn(20))
		spec.endRow = uint32(rng.Intn(3))
	}
	if depth >= 3 {
		return spec
	}
	for i, n := 0, rng.Intn(3); i < n; i++ {
		spec.children = append(spec.children, randomElectionSpec(rng, depth+1))
	}
	return spec
}
