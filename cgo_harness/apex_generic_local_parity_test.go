//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestApexCloseAngleRawCOracleParity(t *testing.T) {
	goLang := grammars.ApexLanguage()
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
			name: "nested_generic_local",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    List<List<SObject>> searchResults = [FIND :keyword IN ALL FIELDS];\n" +
				"  }\n" +
				"}\n"),
		},
		{
			name: "right_shift",
			source: []byte("public class C {\n" +
				"  Integer m(Integer value) {\n" +
				"    return value >> 1;\n" +
				"  }\n" +
				"}\n"),
		},
		{
			// Former witness for issue #601: both class_literal and
			// field_access used to reach acceptance as live parallel
			// derivations (a grammar-declared conflict with no
			// prec.dynamic tie-break), and gotreesitter's runtime
			// accept-selection elected class_literal where the locked C
			// oracle elects field_access. Fixed natively: a per-stack DFA
			// relex (parser.go) stops the shared lexer from starving the
			// field_access fork, and the certified primary-acceptance-
			// derivation preference (parser_result.go, reusing
			// CompactPrimaryAcceptanceDerivationCertified) now settles the
			// resulting tie the same way the compact route already did.
			name: "class_literal_alias",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    Object t = RecordPage.class;\n" +
				"  }\n" +
				"}\n"),
		},
	}

	cLang, err := COracleLanguage("apex")
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
					t.Fatalf("expected %q to diverge from the C oracle (issue #601: class_literal vs field_access election), but the raw trees now match; the underlying election fix landed -- flip wantDivergence to false and re-verify before shipping", test.name)
				}
				t.Skipf("known divergence from the C oracle (issue #601), gotreesitter's runtime accept-selection still elects class_literal over field_access:\n%s", strings.Join(mismatches, "\n"))
				return
			}
			if len(mismatches) != 0 {
				t.Fatalf("raw Go and C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
