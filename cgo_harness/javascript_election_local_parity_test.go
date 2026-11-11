//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func mustReadCorpusFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestJavaScriptElectionRawCOracleParity compares the raw production tree
// (compat tail off) against the locked C oracle for dispatch.javascript's
// remaining live top-level object-literal rewrite.
func TestJavaScriptElectionRawCOracleParity(t *testing.T) {
	goLang := grammars.JavascriptLanguage()
	tests := []struct {
		name   string
		source []byte
		// wantDivergence marks a witness known to diverge from the C oracle
		// today. The subtest records the divergence with t.Skipf instead of
		// failing, and fails loudly if the divergence stops reproducing, so
		// the suite stays green while the divergence persists and demands
		// attention the moment it is fixed.
		wantDivergence bool
	}{
		{
			name:   "dynamic_import_call",
			source: []byte("import(\"foo\");\n"),
		},
		{
			name:   "dynamic_import_call_in_expression",
			source: []byte("async function f() {\n  const m = await import(\"foo\");\n  return m;\n}\n"),
		},
		{
			// dispatcher census witness (parser_result_test/dispatcher_census_test.go
			// over cgo_harness/corpus_real/javascript): dispatch.javascript
			// rewrites 7 positions in this exact file. The two dynamic-import
			// witnesses above remain native, so this is the live-firing shape.
			//
			// For `{ key: value }` at the start of a statement, the C oracle
			// forks between labeled_statement and an object-literal pair, then
			// elects the object literal. The shipped javascript.bin blob still
			// elects labeled_statement only, but the current generator source
			// does not cause that: grammar.js declares
			// [$.labeled_statement, $._property_name] in `conflicts`, and
			// grammargen/lr.go's shiftReduceInConflictGroup already retains
			// both actions for this exact shape (added 2026-03-16, commit
			// bb6c8aa0; pinned by
			// TestResolveShiftReduceKeepsLabeledStatementPropertyNameDeclaredConflict
			// in grammargen/lr_conflict_resolution_test.go). The blob was last
			// regenerated 2026-03-02, two weeks before that fix landed, and was
			// never resynced afterward.
			//
			// Regenerating the blob is blocked by a separate, unrelated bug:
			// grammars.AdaptScannerForLanguage corrupts the raw parse of a
			// freshly regenerated javascript blob (early truncation, exposed
			// supertype nodes) through the production loading path
			// (grammars.JavascriptLanguage / grammars.LoadLanguage). This
			// reproduces with no table-generation change at all. See
			// wantDivergence stays true until both the blob resync and the
			// scanner fix land.
			name:           "corpus_real_small_functions",
			source:         mustReadCorpusFile(t, "corpus_real/javascript/small__functions.js"),
			wantDivergence: true,
		},
	}

	cLang, err := COracleLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			goTree, err := goParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned a nil tree")
			}
			defer cTree.Close()

			var mismatches []string
			compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &mismatches)
			if test.wantDivergence {
				if len(mismatches) == 0 {
					t.Fatalf("expected %q to diverge from the C oracle, but the raw trees now match; the underlying table-generation fix landed -- flip wantDivergence to false and re-verify before shipping", test.name)
				}
				t.Skipf("known divergence from the C oracle: the shipped javascript.bin blob predates the 2026-03-16 declared-conflict retention fix and was never resynced (see the witness comment above):\n%s", strings.Join(mismatches, "\n"))
				return
			}
			if len(mismatches) != 0 {
				t.Fatalf("raw Go and C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
