//go:build !gts_no_parsercorephase0 && gts_derivation_set_census

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdmissionCandidateCSharpMaterialityNonVacuityContract proves the real
// C# row reaches eight live candidates and selects fold zero without a
// materiality decline. This is a non-vacuity contract, not a diagnostic
// receipt.
func TestAdmissionCandidateCSharpMaterialityNonVacuityContract(t *testing.T) {
	source := csharpSmallVariableDeclarationsBytes(t)
	if !gts.DerivationSetCensusBuilt() {
		t.Fatal("derivation-set census is not built")
	}
	gts.DerivationSetCensusReset()
	t.Cleanup(gts.DerivationSetCensusReset)
	gts.ResetAdmissionCandidateCountersForTest()

	parser := gts.NewParser(grammars.CSharpLanguage())
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("candidate counters = %d/%d, want 1/0; reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}

	var final []gts.DerivationSetCensusAccept
	for _, accept := range gts.DerivationSetCensusSnapshot() {
		if accept.ByteOffset == uint32(len(source)) {
			final = append(final, accept)
		}
	}
	if len(final) != 1 {
		t.Fatalf("final C# census accepts = %d, want one: %+v", len(final), final)
	}
	accept := final[0]
	if accept.EnumerationTruncated || accept.DeclinedMaterial {
		t.Fatalf("final C# census declined or truncated: %+v", accept)
	}
	if len(accept.Candidates) != 8 {
		t.Fatalf("final C# census candidates = %d, want eight: %+v", len(accept.Candidates), accept.Candidates)
	}
	if accept.SelectedFoldIndex != 0 {
		t.Fatalf("final C# selected fold = %d, want deterministic fold zero", accept.SelectedFoldIndex)
	}

	shape := accept.Candidates[0].Shape
	for index, candidate := range accept.Candidates {
		if candidate.FoldIndex != index || candidate.Score != 1 || !candidate.HasBranchOrder {
			t.Fatalf("C# candidate %d = %+v, want fold order, score one, and a branch order", index, candidate)
		}
		if candidate.MaterializeError != "" || candidate.Shape != shape {
			t.Fatalf("C# candidate %d materialization = error %q shape %q, want no error and the common shape %q", index, candidate.MaterializeError, candidate.Shape, shape)
		}
	}
}
