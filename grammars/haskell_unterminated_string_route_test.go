//go:build !gts_no_parsercorephase0

package grammars_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHaskellUnterminatedStringUsesInternalGapFallback(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	source := []byte("\"\n")

	production := gotreesitter.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	root := productionTree.RootNode()
	if got := root.SExpr(lang); got != "(haskell (ERROR))" {
		t.Fatalf("production tree=%s, want (haskell (ERROR))", got)
	}
	if !root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		t.Fatalf("production root hasError=%t span=%d..%d, want error covering 0..%d",
			root.HasError(), root.StartByte(), root.EndByte(), len(source))
	}

	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	candidate := gotreesitter.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()
	if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != beforeRouted || fallback != beforeFallback+1 {
		t.Fatalf("route counters routed=%d fallback=%d, want routed=%d fallback=%d",
			routed, fallback, beforeRouted, beforeFallback+1)
	}
	if reason := gotreesitter.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "accepted-leaf-tiling-gap") {
		t.Fatalf("fallback reason=%q, want accepted-leaf-tiling-gap", reason)
	}
	if got, want := candidateTree.RootNode().SExpr(lang), root.SExpr(lang); got != want {
		t.Fatalf("served tree=%s, want production tree=%s", got, want)
	}
}
