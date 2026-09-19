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
		{
			// A3 certification workstream (spec.campaign.v7, finding
			// tied-election-family-compact-retirement): the tied push-list
			// election real-corpus witness
			// (cgo_harness/corpus_real/perl/medium__unicode_ranges.pl).
			name:   "perl",
			source: "push @found, $_;\n",
			load:   grammars.PerlLanguage,
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

func TestAdmissionCandidateSelectedLineageSplitsMatchProduction(t *testing.T) {
	tests := []struct {
		name string
		load func() *gts.Language
	}{
		{name: "dart", load: grammars.DartLanguage},
		{name: "elixir", load: grammars.ElixirLanguage},
		{name: "scala", load: grammars.ScalaLanguage},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(grammars.ParseSmokeSample(test.name))
			lang := test.load()
			if lang.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("selected-lineage witness unexpectedly has an artifact certificate")
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

// TestAdmissionCandidatePlatformModifierWitnessNowClean documents the
// retirement of the former
// TestAdmissionCandidateKotlinPlatformModifierSplitDeclinesWithSplitDropsWithheld
// regression witness. That test pinned a compact-route divergence: the
// visibility-modifier and identifier conflict paths used to merge, then
// split during a later reduction, and production carried a stale
// issue #93-adjacent bug (an ERROR tree containing infix_expression) for
// "internal actual fun f(): String = \"x\"\n".
//
// fwcd/tree-sitter-kotlin@1852ea17b7f6 ("Prefer class and object
// declarations over infix expressions", #280) fixed the underlying grammar
// ambiguity upstream. Verified against the locked C oracle
// (cgo_harness/b4b_alternative_set_v2_kotlin_adjudication_test.go,
// TestB4bAlternativeSetV2KotlinWitnessCOracleAdjudication): production is
// now C-exact on this witness, and the compact route -- even with
// CompactConvergedReductionSplitDropsCertified forced on -- agrees with
// production and the C oracle. There is no more divergence to decline or
// fall back from, so this witness no longer exercises the converged-path
// split fallback. TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe
// (admission_switch_kotlin_certification_test.go) still pins the exact
// tree shape for the forced-split-drops route.
func TestAdmissionCandidatePlatformModifierWitnessNowClean(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
	lang := grammars.KotlinLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
	}

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionSExpr := productionTree.RootNode().SExpr(lang)
	if productionTree.RootNode().HasError() || strings.Contains(productionSExpr, "infix_expression") {
		t.Fatalf("production regressed to the pre-1852ea17b7f6 issue #93-adjacent bug: %s", productionSExpr)
	}
	want := "(source_file (function_declaration (modifiers (visibility_modifier) (platform_modifier)) " +
		"(simple_identifier) (function_value_parameters) (user_type (type_identifier)) " +
		"(function_body (string_literal (string_content)))))"
	if productionSExpr != want {
		t.Fatalf("production tree = %s, want %s", productionSExpr, want)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	// No certification is required to reach the right tree on this witness
	// any more: the compact route either routes cleanly or falls back, and
	// either way it must agree with production.
	if candidateSExpr := candidateTree.RootNode().SExpr(lang); candidateSExpr != productionSExpr {
		t.Fatalf("candidate tree diverged:\nproduction=%s\ncandidate=%s", productionSExpr, candidateSExpr)
	}
}

func TestAdmissionCandidateSelectedLineageJuliaFailsClosed(t *testing.T) {
	const (
		sourcePath   = "testdata/compact_selected_lineage/julia_utils.jl"
		sourceSHA256 = "d81017a2d640f6c84f2ca2a7030687049b7334bdb75ad5c50302b29052ecf79c"
	)
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read Julia selected-lineage witness: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sourceSHA256 {
		t.Fatalf("Julia selected-lineage witness SHA-256 = %s, want %s", got, sourceSHA256)
	}
	lang := grammars.JuliaLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("Julia unexpectedly has an artifact certificate")
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
	if routed != 0 || fallback != 1 {
		t.Fatalf("Julia selected-lineage witness did not fail closed: routed=%d fallback=%d", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "converged-path reduction split") {
		t.Fatalf("fallback reason=%q", reason)
	}
	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect production tree: %v", err)
	}
	candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect fallback tree: %v", err)
	}
	if candidateInspection.SHA256 != productionInspection.SHA256 {
		t.Fatalf(
			"fallback digest %s differs from production %s",
			candidateInspection.SHA256,
			productionInspection.SHA256,
		)
	}
}
