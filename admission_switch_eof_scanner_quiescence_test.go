//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// scalaEOFScannerQuiescenceWitnesses are the two Scala sources that reach the
// compact end-of-input admission frontier: one accepting head and one
// no-action sibling, with both heads carrying the scanner's trailing
// zero-width _automatic_semicolon.
//
// The smoke sample forks once, at end of input. The multi-statement witness
// carries seven more zero-width external tokens before the fork
// (_automatic_semicolon, _indent, _outdent, _control_tail_gate), so it
// exercises the shared-history check rather than the tail alone.
var scalaEOFScannerQuiescenceWitnesses = []struct {
	name   string
	source string
}{
	{
		name:   "smoke",
		source: "object Main { def f(x: Int): Int = x + 1 }\n",
	},
	{
		name: "multi_statement",
		source: "import foo.bar.Baz\n" +
			"object Outer {\n" +
			"  private def search(value: Int): Int =\n" +
			"    if value == 0 then 1 else 2\n" +
			"}\n",
	},
}

// TestAdmissionCandidateScalaEOFScannerQuiescenceRoutesDirectly pins the
// end-of-input scanner quiescence admission.
//
// tree-sitter-scala db390f312a54 externalizes operator precedence, so both
// witnesses reach true end of input with two live heads: one reduced to
// compilation_unit and holds a sole Accept row, one still holds the open
// object header and has no action at all. Both heads already shifted the same
// zero-width _automatic_semicolon, so the fork is an ordinary grammar
// conflict above the scanner.
//
// C resolves that shape by error cost. ts_parser__advance accepts the first
// version and pauses the second, ts_parser__condense_stack resumes the paused
// version into ts_parser__handle_error at a cost above zero, and
// ts_parser__select_tree keeps the accepted cost-zero tree. The compact
// admission models the same rule, and
// proveCompactEOFScannerQuiescence (parsercore_phase0_eof_scanner_quiescence.go)
// supplies the missing scanner proof: it re-runs the external scanner once per
// head state, in isolation, and requires the same authenticated end-of-input
// token and an unchanged serialized scanner state from every run.
//
// The route must serve the tree directly (routed=1, fallback=0) and the tree
// must equal production's byte for byte. The C-oracle counterpart lives in
// cgo_harness/scala_eof_scanner_quiescence_oracle_test.go.
func TestAdmissionCandidateScalaEOFScannerQuiescenceRoutesDirectly(t *testing.T) {
	lang := grammars.ScalaLanguage()
	if lang.ExternalScanner == nil {
		t.Fatal("scala lost its external scanner, so this witness no longer exercises the proof")
	}
	for _, witness := range scalaEOFScannerQuiescenceWitnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			if productionTree.RootNode().HasError() {
				t.Fatal("production produced an error tree, so this witness is not a clean fork")
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
			if routed != 1 || fallback != 0 {
				t.Fatalf(
					"candidate route counters = %d/%d, want 1/0; reason=%s",
					routed,
					fallback,
					gts.AdmissionCandidateLastFallbackReason(),
				)
			}
			if candidateTree.RootNode().HasError() {
				t.Fatal("the compact route accepted an error tree")
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

// TestAdmissionCandidateEOFScannerQuiescenceKeepsScannerLanguagesFailClosed
// checks that the proof widened one frontier shape and nothing else.
//
// Every language below owns an external scanner and serves its smoke sample
// through the compact route today, so each one pins routed=1 and fallback=0
// rather than accepting either outcome: a digest check alone would pass even
// if the whole set silently moved to production. None of the nine reaches the
// end-of-input recovery admission at all. Measured prover calls, one parse
// each: Bash, Go, Kotlin, Python, Ruby 0; Dart, Elixir, Haskell, TypeScript 0;
// Scala 1. They route through the ordinary sole-accept frontier, so the
// eof-scanner-quiescence census bucket stays empty for all nine.
// TestEOFRecoveryAdmissionCensusRecordsScannerQuiescenceMechanism pins that
// for Go through the admission census itself.
func TestAdmissionCandidateEOFScannerQuiescenceKeepsScannerLanguagesFailClosed(t *testing.T) {
	// This test loads nine grammars at once. Release them afterwards, exactly
	// as TestAdmissionCandidateScorecard206 does (admission_scorecard_test.go):
	// the root race shards run every test in one process, and a retained
	// multi-grammar heap raises garbage-collection pauses for later tests that
	// assert a wall-clock parse budget.
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	languages := map[string]func() *gts.Language{
		"bash":       grammars.BashLanguage,
		"dart":       grammars.DartLanguage,
		"elixir":     grammars.ElixirLanguage,
		"go":         grammars.GoLanguage,
		"haskell":    grammars.HaskellLanguage,
		"kotlin":     grammars.KotlinLanguage,
		"python":     grammars.PythonLanguage,
		"ruby":       grammars.RubyLanguage,
		"typescript": grammars.TypescriptLanguage,
	}
	for name, load := range languages {
		name, load := name, load
		t.Run(name, func(t *testing.T) {
			lang := load()
			if lang.ExternalScanner == nil && lang.ExternalTokenCount == 0 {
				t.Skipf("%s declares no external tokens", name)
			}
			source := []byte(grammars.ParseSmokeSample(name))
			if len(source) == 0 {
				t.Skipf("%s has no smoke sample", name)
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
					"%s route counters = %d/%d, want 1/0; reason=%s",
					name, routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
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
					"%s candidate digest %s differs from production %s (routed=%d fallback=%d)",
					name,
					candidateInspection.SHA256,
					productionInspection.SHA256,
					routed,
					fallback,
				)
			}
		})
	}
}
