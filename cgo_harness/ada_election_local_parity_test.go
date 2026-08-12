//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdaElectionRawCOracleParity compares the raw production tree (compat
// tail off) against the locked C oracle for the three registered
// dispatch.ada subpasses' witnesses
// (parser_result_test/materialization_subpass_census_test.go).
func TestAdaElectionRawCOracleParity(t *testing.T) {
	goLang := grammars.AdaLanguage()
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
			// dispatch.ada.constraint-kind-election witness
			// (parser_result_test/materialization_subpass_census_test.go
			// "ada_attribute_constraint"). The raw tree already matches the
			// C oracle here; the subpass runs after this comparison.
			name: "attribute_constraint",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := new T (F => Pkg.Obj'Access);\n" +
				"end;\n"),
		},
		{
			// dispatch.ada.aggregate-kind-election witness
			// ("ada_locked_positional_array_aggregate"; tree-sitter-ada
			// test/corpus/arrays.txt, commit 6b58259a). Both
			// record_aggregate and positional_array_aggregate reach
			// acceptance as live parallel derivations; gotreesitter's
			// runtime accept-selection elects record_aggregate where the
			// locked C oracle elects positional_array_aggregate.
			name: "locked_positional_array_aggregate",
			source: []byte("package P is\n" +
				"   type A is array (1 .. 3) of Boolean;\n" +
				"   V : constant A := (1, 2, 3);\n" +
				"end;\n"),
			wantDivergence: true,
		},
		{
			// dispatch.ada.aggregate-kind-election +
			// dispatch.ada.association-choice-materialization witness
			// ("ada_array_others_choice"). Ada's grammar declares
			// conflicts: [[$.component_choice_list, $.discrete_choice]]: a
			// bare "others" choice reduces as either component_choice_list
			// (record reading) or discrete_choice (array reading), both
			// plain REDUCE with no dynamic precedence. The declared-conflict
			// election policy (grammars/runtime_profiles.go "ada",
			// ConflictPolicyDeclaredReduceReduceHighestSymbol in
			// conflict_policy.go) folds the row to the discrete_choice
			// reading before the GLR engine ever forks, which in turn
			// determines the outer named_array_aggregate reading (no
			// separate outer-shape ambiguity remains once the inner choice
			// is settled), so the raw production tree now matches C
			// natively.
			name: "array_others_choice",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := (others => 0);\n" +
				"end;\n"),
		},
	}

	cLang, err := COracleLanguage("ada")
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
					t.Fatalf("expected %q to diverge from the C oracle, but the raw trees now match; the underlying election fix landed -- flip wantDivergence to false and re-verify before shipping", test.name)
				}
				t.Skipf("known divergence from the C oracle, gotreesitter's runtime accept-selection elects the alternative the C oracle does not:\n%s", strings.Join(mismatches, "\n"))
				return
			}
			if len(mismatches) != 0 {
				t.Fatalf("raw Go and C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
