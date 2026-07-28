//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
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
		sourceSHA  string
		treeSHA    string
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
			// Source repository: https://github.com/tree-sitter/tree-sitter-javascript
			// Source commit/path: 58404d8cf191d69f2674a8fd507bd5776f46cb11 test/tags/functions.js.
			sourceSHA: "0bbd2cdb0a0492055e442c44b533797386ec9c8aeb7ce8a4d0f5f5a4681e3b90",
			treeSHA:   "75429585a56be767f37a5ea2ee5de028a9947c34ae38d35d9699bb1f2d0133fd",
			load:      grammars.JavascriptLanguage,
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
			if test.sourceSHA != "" {
				if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.sourceSHA {
					t.Fatalf("source SHA-256 = %s, want %s", got, test.sourceSHA)
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
			if test.treeSHA != "" && candidateInspection.SHA256 != test.treeSHA {
				t.Fatalf("candidate digest = %s, want %s", candidateInspection.SHA256, test.treeSHA)
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
