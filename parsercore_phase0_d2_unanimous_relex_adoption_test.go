//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// d2AdoptionUnanimousWitnessSource ("/<(a*?#", swift) is D2-2a's positive
// witness: at byte 5, every live no-action head shares one byte-identical,
// live-action relex (a narrower "?" token in place of the shared "?#"
// external-scanner token). Found by a targeted random sweep of the swift
// alphabet (seed 20260827, 24000 samples, one hit) run during the D2-2
// investigation, not hand-crafted.
var d2AdoptionUnanimousWitnessSource = []byte("/<(a*?#")

// TestD2AdoptionSwiftUnanimousWitnessMovesTerminalStop is the D2-2a positive
// arm: with unanimous relex adoption on (the zero-value default), the
// scheduler adopts the narrower relex at byte 5 instead of declining, then
// reaches a genuine no-table-action decline one byte later, at byte 6. The
// adoption itself is pinned by the work counter, not just the moved byte
// offset.
func TestD2AdoptionSwiftUnanimousWitnessMovesTerminalStop(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, d2AdoptionUnanimousWitnessSource)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance != nil {
		t.Fatal("compact scheduler unexpectedly accepted the adoption witness")
	}
	if receipt.Stop.Boundary != gts.DiagnosticParserCoreRecovery {
		t.Fatalf("terminal Stop.Boundary = %q, want %q", receipt.Stop.Boundary, gts.DiagnosticParserCoreRecovery)
	}
	if want := gts.DiagnosticParserCoreNoTableActionDetailForTest(); receipt.Stop.Detail != want {
		t.Fatalf("terminal Stop.Detail = %q, want %q (adoption moved past the ragged-end decline)", receipt.Stop.Detail, want)
	}
	if receipt.Stop.ByteOffset != 6 {
		t.Fatalf("terminal Stop.ByteOffset = %d, want 6", receipt.Stop.ByteOffset)
	}
	if got := receipt.Stop.Work.RelexAdoptions; got != 1 {
		t.Fatalf("Stop.Work.RelexAdoptions = %d, want 1", got)
	}
}

// TestD2AdoptionDisabledOptionReproducesRaggedDeclineExactly is the D2-2a
// negative arm: DisableUnanimousRelexAdoption on the same witness restores
// the D2-1 ragged-end decline exactly, at byte 5 (one byte before the
// adopted run's own terminal stop), with zero adoptions recorded.
func TestD2AdoptionDisabledOptionReproducesRaggedDeclineExactly(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	receipt, err := gts.RunStateDependentRelexSchedulerWithUnanimousRelexAdoptionDisabledForTest(lang, d2AdoptionUnanimousWitnessSource)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance != nil {
		t.Fatal("compact scheduler unexpectedly accepted the adoption witness")
	}
	if receipt.Stop.Boundary != gts.DiagnosticParserCoreRecovery {
		t.Fatalf("terminal Stop.Boundary = %q, want %q", receipt.Stop.Boundary, gts.DiagnosticParserCoreRecovery)
	}
	wantPrefix := gts.DiagnosticParserCoreRaggedRelexDeclineDetailForTest()
	if !strings.HasPrefix(receipt.Stop.Detail, wantPrefix) {
		t.Fatalf("terminal Stop.Detail = %q, want prefix %q (the D2-1 ragged-end decline)", receipt.Stop.Detail, wantPrefix)
	}
	if receipt.Stop.ByteOffset != 5 {
		t.Fatalf("terminal Stop.ByteOffset = %d, want 5", receipt.Stop.ByteOffset)
	}
	if got := receipt.Stop.Work.RelexAdoptions; got != 0 {
		t.Fatalf("Stop.Work.RelexAdoptions = %d, want 0 (adoption disabled)", got)
	}
}

// d2AdoptionConvergedPathGapWitnessSource ("a<A<#-", swift) is D2-2a's
// second recorded witness (spore.2026-08-27.rowan.d2-2a-unanimous-relex-
// adoption). It is also swiftRaggedEndWitnessSource in
// parsercore_phase0_d2_span_unlocked_relex_test.go, D2-1's own ragged-end
// regression witness: with adoption on, every guard holds here too, so the
// scheduler adopts past D2-1's own decline and a later pass reaches a
// separate, already-known converged-path alternative-set coverage gap (B4b:
// "converged-path reduction split no-action drop lacks alternative-set
// coverage by one non-blended survivor", the same gap
// TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch pins as a
// pre-existing, unrelated branch failure). B4b is a distinct, separately
// filed defect; D2-2a does not fix it here. The requirement this test pins
// is narrower: adoption must never turn that gap into an error the public
// Parse API surfaces to a caller -- the compact route must still decline
// internally and fall back to the production parser cleanly.
var d2AdoptionConvergedPathGapWitnessSource = []byte("a<A<#-")

// TestD2AdoptionSecondWitnessFallsBackThroughConvergedPathGap pins the B4b
// gap (see the witness doc comment above) at the low-level scheduler harness
// and proves the public Parse API still falls back cleanly, never erroring
// to the caller, despite adoption changing which internal decline this
// witness reaches.
func TestD2AdoptionSecondWitnessFallsBackThroughConvergedPathGap(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	// Low-level harness: the internal decline is a Go error here (the B4b
	// no-action drop declines through dropGenericNoActionHeads, which
	// returns an error, not a Stop), so the low-level assertion is on err
	// itself, not receipt.Stop.
	wantErrSubstring := "converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor"
	if _, err := gts.RunStateDependentRelexSchedulerForTest(lang, d2AdoptionConvergedPathGapWitnessSource); err == nil ||
		!strings.Contains(err.Error(), wantErrSubstring) {
		t.Fatalf("compact scheduler err = %v, want an error containing %q (the B4b gap)", err, wantErrSubstring)
	}

	// Public API: Parse must still fall back cleanly, never returning that
	// internal decline as an error to the caller.
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCountersForTest()
	tree, err := parser.Parse(d2AdoptionConvergedPathGapWitnessSource)
	if err != nil {
		t.Fatalf("Parse returned an error instead of falling back: %v", err)
	}
	if tree == nil {
		t.Fatal("Parse returned a nil tree")
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	wantFallbackReason := "compact route declined at no_action: " + wantErrSubstring
	if routed != 0 || fallback != 1 || gts.AdmissionCandidateLastFallbackReason() != wantFallbackReason {
		t.Fatalf("candidate route counters=%d/%d reason=%q, want 0/1 and %q",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(), wantFallbackReason)
	}

	prod := gts.NewParser(lang)
	prod.SetAdmissionCandidateRoute(false)
	prodTree, err := prod.Parse(d2AdoptionConvergedPathGapWitnessSource)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prodTree.Release()
	if prodTree.RootNode().SExpr(lang) != tree.RootNode().SExpr(lang) {
		t.Fatal("fallback tree does not match the production tree")
	}
}

// TestD2AdoptionCPartCTilingDataPinDeferred records the C-tiling data-pin
// deferral (Part B of the D2-2a task): the container-generated manifest
// requires the pinned Docker toolchain this local host does not have,
// so it is not generated or fabricated here. The orchestrator runs that
// gate separately, outside this test.
func TestD2AdoptionCPartCTilingDataPinDeferred(t *testing.T) {
	t.Skip("C-tiling data pin (Part B) deferred to the pinned-container orchestrator run; not generated locally")
}
