//go:build gts_diag && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagnosticBuildContinuesPastCompactProofDecline(t *testing.T) {
	t.Setenv("GTS_DIAG_BYPASS_CONVERGED_SPLIT_NO_ACTION_PROOFS", "1")
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "256")

	entry := grammars.DetectLanguageByName("c_sharp")
	if entry == nil || entry.Language() == nil {
		t.Fatal("c_sharp grammar is unavailable")
	}
	parser := gts.NewParser(entry.Language())
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCounters()
	tree, err := parser.Parse(loadCSharpCliffFixture(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	routed, fallbacks := gts.AdmissionCandidateCounters()
	if routed+fallbacks != 1 {
		t.Fatalf("candidate route counts routed=%d fallbacks=%d, want one attempt", routed, fallbacks)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if strings.Contains(reason, "lacks alternative-set coverage by one non-blended survivor") ||
		strings.Contains(reason, "descends from an unproved historical boundary resurrection") {
		t.Fatalf("diagnostic build stopped at a bypassed proof gate: %q", reason)
	}
}
