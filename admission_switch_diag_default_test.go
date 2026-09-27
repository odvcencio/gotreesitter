//go:build !gts_diag && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDefaultBuildKeepsCompactProofGateAndRoute(t *testing.T) {
	t.Setenv("GTS_ADMISSION_CANDIDATE", "0")
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "256")

	entry := grammars.DetectLanguageByName("c_sharp")
	if entry == nil || entry.Language() == nil {
		t.Fatal("c_sharp grammar is unavailable")
	}
	defaultParser := gts.NewParser(entry.Language())
	gts.ResetAdmissionCandidateCounters()
	defaultTree, err := defaultParser.Parse(loadCSharpCliffFixture(t))
	if err != nil {
		t.Fatalf("default-route parse: %v", err)
	}
	if defaultTree == nil || defaultTree.RootNode() == nil {
		t.Fatal("ordinary default route returned no tree")
	}
	defaultTree.Release()
	if routed, fallbacks := gts.AdmissionCandidateCounters(); routed != 0 || fallbacks != 0 {
		t.Fatalf("ordinary default route counters routed=%d fallbacks=%d, want 0 and 0", routed, fallbacks)
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
	if routed != 0 || fallbacks != 1 {
		t.Fatalf("default route counts routed=%d fallbacks=%d, want 0 and 1", routed, fallbacks)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if !strings.Contains(reason, "declined at no_action:") ||
		!strings.Contains(reason, "lacks alternative-set coverage by one non-blended survivor") {
		t.Fatalf("default build did not preserve the C# proof-gate decline: %q", reason)
	}
	if tree.ParseRuntime().StopReason != gts.ParseStopAccepted || tree.RootNode() == nil {
		t.Fatalf("fallback parse did not preserve the legacy result: stop=%s root=%v", tree.ParseRuntime().StopReason, tree.RootNode())
	}
}
