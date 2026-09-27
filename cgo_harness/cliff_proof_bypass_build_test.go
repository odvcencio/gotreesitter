//go:build linux && cgo && treesitter_c_parity && !gts_no_parsercorephase0

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCliffProofBypassBuildContract(t *testing.T) {
	fixture := cliffFixtures[0]
	source := loadCliffSource(t, fixture)
	armCliffDiagnosticBuildTest(t)
	entry := grammars.DetectLanguageByName(fixture.Grammar)
	if entry == nil || entry.Language() == nil {
		t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
	}
	defaultParser := gts.NewParser(entry.Language())
	defaultParser.SetAdmissionCandidateRoute(false)
	gts.ResetAdmissionCandidateCounters()
	defaultTree, err := defaultParser.Parse(source)
	if err != nil {
		t.Fatalf("default route parse: %v", err)
	}
	if defaultTree == nil || defaultTree.RootNode() == nil {
		t.Fatal("default route returned no tree")
	}
	defaultTree.Release()
	if routed, fallbacks := gts.AdmissionCandidateCounters(); routed != 0 || fallbacks != 0 {
		t.Fatalf("default route counters routed=%d fallbacks=%d, want 0 and 0", routed, fallbacks)
	}

	parser := gts.NewParser(entry.Language())
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("candidate route parse: %v", err)
	}
	defer tree.Release()
	if tree.RootNode() == nil || tree.ParseRuntime().StopReason != gts.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatalf("candidate result is not accepted and full-span: stop=%s root=%v", tree.ParseRuntime().StopReason, tree.RootNode())
	}
	if tree.HasError() {
		t.Fatal("C# candidate result unexpectedly has an error")
	}
	routed, fallbacks := gts.AdmissionCandidateCounters()
	if routed+fallbacks != 1 {
		t.Fatalf("candidate route counts routed=%d fallbacks=%d, want one attempt", routed, fallbacks)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	proofDecline := "lacks alternative-set coverage by one non-blended survivor"
	if core.CompactConvergedSplitProofBypassEnabled {
		if strings.Contains(reason, proofDecline) {
			t.Fatalf("diagnostic bypass stopped at the proof gate: %q", reason)
		}
		if !strings.Contains(reason, "live-link cap exceeded: 9 > 8") {
			t.Fatalf("diagnostic bypass did not reach the next independent cap: %q", reason)
		}
	} else if !strings.Contains(reason, proofDecline) {
		t.Fatalf("ordinary build changed the C# proof-gate decline: %q", reason)
	}
}

func TestCliffHTMLRecoveryDecline(t *testing.T) {
	fixture := cliffFixtures[6]
	source := loadCliffSource(t, fixture)
	armCliffDiagnosticBuildTest(t)
	entry := grammars.DetectLanguageByName(fixture.Grammar)
	if entry == nil || entry.Language() == nil {
		t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
	}
	parser := gts.NewParser(entry.Language())
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("candidate route parse: %v", err)
	}
	if tree != nil {
		tree.Release()
	}
	if routed, fallbacks := gts.AdmissionCandidateCounters(); routed != 0 || fallbacks != 1 {
		t.Fatalf("HTML route counts routed=%d fallbacks=%d, want recovery decline and one fallback", routed, fallbacks)
	}
	want := "compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
	if got := gts.AdmissionCandidateLastFallbackReason(); got != want {
		t.Fatalf("HTML recovery decline = %q, want %q", got, want)
	}
	t.Log(want)
}

func armCliffDiagnosticBuildTest(t *testing.T) {
	t.Helper()
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "256")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	gts.ResetAdmissionCandidateCounters()
	t.Cleanup(gts.ResetAdmissionCandidateCounters)
}
