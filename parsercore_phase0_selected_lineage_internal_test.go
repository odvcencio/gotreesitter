//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreSelectedLineageDropProof(t *testing.T) {
	tests := []struct {
		name    string
		headers []diagnosticParserCoreHeader
		indices []int
		count   uint64
		proved  bool
	}{
		{
			name: "unselected-to-selected",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7},
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnselected, cleanPathLineage: 7},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			name: "selected-drop",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7},
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnselected, cleanPathLineage: 7},
			},
			indices: []int{0},
		},
		{
			name: "unknown-drop",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7},
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnknown},
			},
			indices: []int{1},
		},
		{
			name: "different-lineage",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 8},
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnselected, cleanPathLineage: 7},
			},
			indices: []int{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, proved := diagnosticParserCoreSelectedLineageDrops(test.headers, test.indices)
			if count != test.count || proved != test.proved {
				t.Fatalf("proof = %d/%t, want %d/%t", count, proved, test.count, test.proved)
			}
		})
	}
}

func TestDiagnosticParserCoreSelectedLineageProofAllocations(t *testing.T) {
	headers := []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7},
		{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnselected, cleanPathLineage: 7},
	}
	indices := []int{1}
	if allocations := testing.AllocsPerRun(1000, func() {
		count, proved := diagnosticParserCoreSelectedLineageDrops(headers, indices)
		if count != 1 || !proved {
			panic("selected-lineage proof failed")
		}
	}); allocations != 0 {
		t.Fatalf("warm selected-lineage proof allocations = %g, want 0", allocations)
	}
}

func TestDiagnosticParserCoreExternalShiftInvalidatesLineage(t *testing.T) {
	header := diagnosticParserCoreHeader{
		cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7,
	}
	markDiagnosticParserCoreExternalLineage(&header, Token{ExternalScannerToken: true})
	if header.cleanPathRank != core.CleanPathRankUnknown || header.cleanPathLineage != 0 {
		t.Fatalf("external shift retained selected lineage: %+v", header)
	}
}

// TestDiagnosticParserCoreExternalShiftCarriesAlternativeSet pins the
// opposite property for the new record: unlike the scalar pair above, an
// external shift must never erase altSet (spec.b4b-alternative-set.v1
// section 4, "External shifts: carry, do not erase" -- the F1 fix). The
// scalar pair stays the live decider through stage 2a, but altSet's own
// carry property is unconditional regardless of which proof decides.
func TestDiagnosticParserCoreExternalShiftCarriesAlternativeSet(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	header := diagnosticParserCoreHeader{
		cleanPathRank: core.CleanPathRankSelected, cleanPathLineage: 7,
		altSet: core.NewAlternativeSetMember(7, 0),
	}
	before := header.altSet
	markDiagnosticParserCoreExternalLineage(&header, Token{ExternalScannerToken: true})
	if header.altSet != before {
		t.Fatalf("external shift disturbed the carried alternative set: got %+v, want %+v", header.altSet, before)
	}
}

// alternativeSetPinCoverageTable is a minimal core.TableView stub: the
// converged-coverage predicate below never dispatches through a table, it
// only resolves AlternativeSet membership, so every method is unreachable.
type alternativeSetPinCoverageTable struct{}

func (alternativeSetPinCoverageTable) Actions(core.StateID, core.Symbol) (core.ActionRow, error) {
	return core.ActionRow{}, nil
}
func (alternativeSetPinCoverageTable) Goto(core.StateID, core.Symbol) (core.StateID, error) {
	return 0, nil
}
func (alternativeSetPinCoverageTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}
func (alternativeSetPinCoverageTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func newAlternativeSetPinScheduler(t *testing.T) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	compact, err := core.New(alternativeSetPinCoverageTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return &diagnosticParserCoreGenericScheduler{compact: compact}
}

// alternativeSetPinMember builds a set from bare events, each on branch 0 --
// the v1-equivalent shape (spec.b4b-alternative-set.v1 section 5) -- for
// tests that exercise only event-only containment. compact must be the same
// Core instance the caller will later read the set back through (typically
// scheduler.compact): a spilled set's spillRef only resolves against the
// arena that produced it (core.go's alternativeSetMembers doc comment), so
// building with one Core and reading with another silently fails closed the
// moment a set actually spills. This was previously masked at the pre-b4b-
// width-repair inline capacity (4): every set literal in this file has at
// most 3 members, which never spilled at capacity 4 but does at the audited
// capacity of 2 (core.go), so the aliasing defect is load-bearing now.
func alternativeSetPinMember(t *testing.T, compact *core.Core, members ...uint16) core.AlternativeSet {
	t.Helper()
	var set core.AlternativeSet
	for _, m := range members {
		compact.UnionAlternativeSet(&set, core.NewAlternativeSetMember(m, 0))
	}
	return set
}

// alternativeSetPinBranchMember builds a set from explicit (event, branch)
// pairs, for v2-specific tests. See alternativeSetPinMember's doc comment
// for why compact must be the same Core instance the caller reads through.
func alternativeSetPinBranchMember(t *testing.T, compact *core.Core, pairs ...[2]uint16) core.AlternativeSet {
	t.Helper()
	var set core.AlternativeSet
	for _, pair := range pairs {
		compact.UnionAlternativeSet(&set, core.NewAlternativeSetMember(pair[0], pair[1]))
	}
	return set
}

func TestDiagnosticParserCoreConvergedCoverageDropsV1Proof(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	// One shared scheduler (and compact) builds every test-table set below
	// and later reads each back: diagnosticParserCoreConvergedCoverageDropsV1
	// only reads through scheduler.compact, so sharing it across subtests is
	// safe, and it is required -- see alternativeSetPinMember's doc comment.
	scheduler := newAlternativeSetPinScheduler(t)
	member := func(members ...uint16) core.AlternativeSet {
		return alternativeSetPinMember(t, scheduler.compact, members...)
	}
	tests := []struct {
		name    string
		headers []diagnosticParserCoreHeader
		indices []int
		count   uint64
		proved  bool
	}{
		{
			name: "single-member-containment",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(7)},
				{convergedReductionSplit: true, altSet: member(7)},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			name: "multi-member-superset-containment",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(2, 5, 9)},
				{convergedReductionSplit: true, altSet: member(2, 5)},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			name: "partial-containment-fails-closed",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(2, 9)},
				{convergedReductionSplit: true, altSet: member(2, 5)},
			},
			indices: []int{1},
		},
		{
			// F2 (spec section 2): an exact cross-path tie zeroes the scalar
			// lineage (cleanPathRank stays Unknown) but never zeroes the set,
			// so containment still proves the drop.
			name: "unknown-rank-still-proves-by-set",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(7)},
				{convergedReductionSplit: true, cleanPathRank: core.CleanPathRankUnknown, altSet: member(7)},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			// F1 (spec section 2): an external shift erased cleanPathLineage
			// to zero, but the set was carried, not erased.
			name: "erased-scalar-lineage-still-proves-by-set",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(7)},
				{convergedReductionSplit: true, cleanPathLineage: 0, altSet: member(7)},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			name: "empty-dropped-set-fails-closed",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: member(7)},
				{convergedReductionSplit: true},
			},
			indices: []int{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheduler.headers = test.headers
			count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV1(test.indices)
			if count != test.count || proved != test.proved {
				t.Fatalf("proof = %d/%t, want %d/%t", count, proved, test.count, test.proved)
			}
		})
	}
}

func TestDiagnosticParserCoreConvergedCoverageDropsV1OverflowedDroppedFailsClosed(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler := newAlternativeSetPinScheduler(t)
	dropped := core.NewAlternativeSetMember(1, 0)
	for member := uint16(2); member <= 40; member++ {
		scheduler.compact.UnionAlternativeSet(&dropped, core.NewAlternativeSetMember(member, 0))
	}
	if !dropped.Overflowed() {
		t.Fatal("setup did not overflow the dropped set")
	}
	survivor := dropped // recorded members are a subset either way; still must fail closed
	scheduler.headers = []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, altSet: survivor},
		{convergedReductionSplit: true, altSet: dropped},
	}
	count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV1([]int{1})
	if count != 0 || proved {
		t.Fatalf("proof = %d/%t, want 0/false (overflowed dropped set is incomplete)", count, proved)
	}
}

// TestDiagnosticParserCoreConvergedCoverageDropsV1ProofAllocations is the
// converged-coverage counterpart to
// TestDiagnosticParserCoreSelectedLineageProofAllocations
// (spec.b4b-alternative-set.v1 section 3.4's second new pin).
func TestDiagnosticParserCoreConvergedCoverageDropsV1ProofAllocations(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler := newAlternativeSetPinScheduler(t)
	survivorSet := core.NewAlternativeSetMember(7, 0)
	scheduler.compact.UnionAlternativeSet(&survivorSet, core.NewAlternativeSetMember(9, 0))
	scheduler.headers = []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, altSet: survivorSet},
		{convergedReductionSplit: true, altSet: core.NewAlternativeSetMember(7, 0)},
	}
	indices := []int{1}
	if allocations := testing.AllocsPerRun(1000, func() {
		count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV1(indices)
		if count != 1 || !proved {
			panic("converged-coverage v1 proof failed")
		}
	}); allocations != 0 {
		t.Fatalf("warm converged-coverage v1 proof allocations = %g, want 0", allocations)
	}
}

// TestDiagnosticParserCoreConvergedCoverageDropsV2Proof pins the revised
// theorem's (event, branch) exact-match containment and blended-witness veto
// (spec.b4b-alternative-set.v2 section 5).
func TestDiagnosticParserCoreConvergedCoverageDropsV2Proof(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	// One shared scheduler (and compact) builds every test-table set below
	// and later reads each back -- see
	// TestDiagnosticParserCoreConvergedCoverageDropsV1Proof's identical setup
	// and alternativeSetPinMember's doc comment.
	scheduler := newAlternativeSetPinScheduler(t)
	branchMember := func(pairs ...[2]uint16) core.AlternativeSet {
		return alternativeSetPinBranchMember(t, scheduler.compact, pairs...)
	}
	tests := []struct {
		name    string
		headers []diagnosticParserCoreHeader
		indices []int
		count   uint64
		proved  bool
	}{
		{
			name: "identical-branch-admits",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0})},
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0})},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			// The Kotlin class (spec section 6.1): same event, different
			// branch. v1 event-only identity would admit this; v2 must
			// decline, since neither packed value contains the other.
			name: "differing-branch-same-event-declines",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 1})},
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0})},
			},
			indices: []int{1},
		},
		{
			// A blended survivor cannot serve as a witness even though it
			// exactly contains the dropped member (spec section 3.4/5).
			name: "blended-witness-vetoed",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0}), blended: true},
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0})},
			},
			indices: []int{1},
		},
		{
			// The nested-ambiguity asymmetric-superset case (spec section
			// 6.2): D={(A,1)}, S={(A,1),(B,x)} is safely admitted.
			name: "asymmetric-superset-admits",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{1, 1}, [2]uint16{2, 9})},
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{1, 1})},
			},
			indices: []int{1}, count: 1, proved: true,
		},
		{
			name: "empty-dropped-set-fails-closed",
			headers: []diagnosticParserCoreHeader{
				{convergedReductionSplit: true, altSet: branchMember([2]uint16{5, 0})},
				{convergedReductionSplit: true},
			},
			indices: []int{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheduler.headers = test.headers
			count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV2(test.indices)
			if count != test.count || proved != test.proved {
				t.Fatalf("proof = %d/%t, want %d/%t", count, proved, test.count, test.proved)
			}
		})
	}
}

func TestDiagnosticParserCoreConvergedCoverageDropsV2BlendedVetoFiredDetail(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler := newAlternativeSetPinScheduler(t)
	scheduler.headers = []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, altSet: alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{5, 0}), blended: true},
		{convergedReductionSplit: true, altSet: alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{5, 0})},
	}
	blendedVetoFired, count, proved, overflowDeclined := scheduler.diagnosticParserCoreConvergedCoverageDropsV2Detail([]int{1})
	if !blendedVetoFired {
		t.Fatal("expected the blended veto to fire: an exact-member-containing witness was rejected only for being blended")
	}
	if count != 0 || proved || overflowDeclined {
		t.Fatalf("detail = count=%d proved=%t overflowDeclined=%t, want 0/false/false", count, proved, overflowDeclined)
	}
}

func TestDiagnosticParserCoreConvergedCoverageDropsV2OverflowedDroppedFailsClosed(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler := newAlternativeSetPinScheduler(t)
	dropped := core.NewAlternativeSetMember(1, 0)
	for member := uint16(2); member <= 40; member++ {
		scheduler.compact.UnionAlternativeSet(&dropped, core.NewAlternativeSetMember(member, 0))
	}
	if !dropped.Overflowed() {
		t.Fatal("setup did not overflow the dropped set")
	}
	survivor := dropped
	scheduler.headers = []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, altSet: survivor},
		{convergedReductionSplit: true, altSet: dropped},
	}
	count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV2([]int{1})
	if count != 0 || proved {
		t.Fatalf("proof = %d/%t, want 0/false (overflowed dropped set is incomplete)", count, proved)
	}
}

// TestDiagnosticParserCoreConvergedCoverageDropsV2ProofAllocations is the
// v2 counterpart to TestDiagnosticParserCoreSelectedLineageProofAllocations.
func TestDiagnosticParserCoreConvergedCoverageDropsV2ProofAllocations(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler := newAlternativeSetPinScheduler(t)
	survivorSet := core.NewAlternativeSetMember(7, 0)
	scheduler.compact.UnionAlternativeSet(&survivorSet, core.NewAlternativeSetMember(9, 0))
	scheduler.headers = []diagnosticParserCoreHeader{
		{convergedReductionSplit: true, altSet: survivorSet},
		{convergedReductionSplit: true, altSet: core.NewAlternativeSetMember(7, 0)},
	}
	indices := []int{1}
	if allocations := testing.AllocsPerRun(1000, func() {
		count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV2(indices)
		if count != 1 || !proved {
			panic("converged-coverage v2 proof failed")
		}
	}); allocations != 0 {
		t.Fatalf("warm converged-coverage v2 proof allocations = %g, want 0", allocations)
	}
}
