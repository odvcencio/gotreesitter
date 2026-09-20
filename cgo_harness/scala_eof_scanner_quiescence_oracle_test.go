//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// scalaEOFScannerQuiescenceCOracleRoot loads the locked Scala C oracle and
// parses source, returning its root node and a close func the caller must
// defer.
func scalaEOFScannerQuiescenceCOracleRoot(t *testing.T, source []byte) (*sitter.Node, func()) {
	t.Helper()
	cLang, err := COracleLanguage("scala")
	if err != nil {
		t.Fatalf("load Scala C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		t.Fatalf("set Scala C oracle language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		cParser.Close()
		t.Fatal("C oracle parse returned no root")
	}
	return cTree.RootNode(), func() {
		cTree.Close()
		cParser.Close()
	}
}

// TestScalaEOFScannerQuiescenceCompactAcceptIsCExact is the C-oracle receipt
// for the end-of-input scanner quiescence admission.
//
// tree-sitter-scala db390f312a54 externalizes operator precedence, so both
// witnesses below reach true end of input with two live heads: one reduced to
// compilation_unit with a sole Accept row, one still holding the open object
// header with no action at all. Both heads already shifted the same zero-width
// _automatic_semicolon.
//
// C decides that shape by error cost. ts_parser__advance accepts the first
// version (ts_parser__accept) and pauses the second one ("detect_error");
// ts_parser__condense_stack then resumes the paused version into
// ts_parser__handle_error, whose every outcome costs more than zero, so
// ts_parser__select_tree keeps the accepted cost-zero tree. The compact
// admission models that rule and
// proveCompactEOFScannerQuiescence (parsercore_phase0_eof_scanner_quiescence.go)
// supplies the scanner proof C gets for free by lexing per version.
//
// This test adjudicates the served tree directly against the locked oracle.
// The compact route must accept (routed+1, no fallback) and the accepted tree
// must be C-exact, not merely equal to a second Go parse. The host-side
// counterpart is TestAdmissionCandidateScalaEOFScannerQuiescenceRoutesDirectly
// (admission_switch_eof_scanner_quiescence_test.go).
func TestScalaEOFScannerQuiescenceCompactAcceptIsCExact(t *testing.T) {
	witnesses := []struct {
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

	goLang := grammars.ScalaLanguage()
	if goLang.ExternalScanner == nil {
		t.Fatal("scala lost its external scanner, so these witnesses no longer exercise the proof")
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			cRoot, closeC := scalaEOFScannerQuiescenceCOracleRoot(t, source)
			defer closeC()

			production := gotreesitter.NewParser(goLang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()

			var productionMismatches []string
			compareNodes(productionTree.RootNode(), goLang, cRoot, "root", &productionMismatches)
			if len(productionMismatches) != 0 {
				t.Fatalf(
					"production diverges from the C oracle on this witness (an unrelated "+
						"regression, not the admission this test pins):\n%s",
					strings.Join(productionMismatches, "\n"),
				)
			}

			gotreesitter.ResetAdmissionCandidateCounters()
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			candidate := gotreesitter.NewParser(goLang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			defer candidateTree.Release()

			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf(
					"candidate route counters before=(%d,%d) after=(%d,%d), want routed+1 only; reason=%s",
					routedBefore, fallbackBefore, routedAfter, fallbackAfter,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}

			var candidateMismatches []string
			compareNodes(candidateTree.RootNode(), goLang, cRoot, "root", &candidateMismatches)
			if len(candidateMismatches) != 0 {
				t.Fatalf(
					"the compact route accepted a tree that diverges from the C oracle at %d point(s):\n%s",
					len(candidateMismatches), strings.Join(candidateMismatches, "\n"),
				)
			}
		})
	}
}
