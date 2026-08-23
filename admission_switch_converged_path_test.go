//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAdmissionCandidateCertifiedConvergedPathSplitsMatchProduction(t *testing.T) {
	tests := []struct {
		name       string
		corpusPath string
		source     string
		load       func() *gts.Language
	}{
		{
			name:       "bash",
			corpusPath: filepath.Join("testdata", "compact_converged_split", "bash.sh"),
			load:       grammars.BashLanguage,
		},
		{
			name:       "erlang",
			corpusPath: filepath.Join("testdata", "compact_converged_split", "erlang.erl"),
			load:       grammars.ErlangLanguage,
		},
		{
			name:   "haskell",
			source: grammars.ParseSmokeSample("haskell"),
			load:   grammars.HaskellLanguage,
		},
		{
			name:       "javascript",
			corpusPath: filepath.Join("testdata", "compact_converged_split", "javascript.js"),
			load:       grammars.JavascriptLanguage,
		},
		{
			name:   "python",
			source: "def greet(name):\n    return f\"hello {name}\"\n\nprint(greet(\"world\"))\n",
			load:   grammars.PythonLanguage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			if test.corpusPath != "" {
				var err error
				source, err = os.ReadFile(test.corpusPath)
				if err != nil {
					t.Fatalf("read compact certification witness: %v", err)
				}
			}

			lang := test.load()
			if !lang.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("exact artifact lacks converged-split certification")
			}

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 1 || fallback != 0 {
				t.Fatalf(
					"candidate route counters = %d/%d, want 1/0; reason=%s",
					routed,
					fallback,
					gts.AdmissionCandidateLastFallbackReason(),
				)
			}
			candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("inspect candidate tree: %v", err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("inspect production tree: %v", err)
			}
			if candidateInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf(
					"candidate digest %s differs from production %s",
					candidateInspection.SHA256,
					productionInspection.SHA256,
				)
			}
		})
	}
}

// TestAdmissionCandidateConvergedPathSplitFailsClosed covers a compact-route
// divergence found by the refreshed Kotlin corpus. The visibility-modifier and
// identifier conflict paths merge, then split during a later reduction.
// Production and tree-sitter C recover the identifier path. The compact route
// otherwise drops that path and returns a different clean tree.
func TestAdmissionCandidateConvergedPathSplitFailsClosed(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
	lang := grammars.KotlinLanguage()

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionSExpr := productionTree.RootNode().SExpr(lang)
	if !productionTree.RootNode().HasError() || !strings.Contains(productionSExpr, "infix_expression") {
		t.Fatalf("production/C witness changed: %s", productionSExpr)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 {
		t.Fatalf("converged-path split did not fail closed: routed=%d fallback=%d", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "converged-path reduction split") {
		t.Fatalf("fallback reason=%q", reason)
	}
	if candidateSExpr := candidateTree.RootNode().SExpr(lang); candidateSExpr != productionSExpr {
		t.Fatalf("fallback tree diverged:\nproduction=%s\ncandidate=%s", productionSExpr, candidateSExpr)
	}
}
