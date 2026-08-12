//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestQLSignatureExprElectionRawCOracleParity is the isolated one-grammar C
// parity receipt for the ql signatureExpr election. QL declares
// conflicts: [[$.simpleId, $.className], ...]: an upper-case identifier in
// signature position (`implements Foo`, `implements M::Foo`) reduces the
// shared _upper_id token as either simpleId (building moduleExpr) or
// className (building typeExpr), both live GLR alternatives of
// signatureExpr's choice(typeExpr, moduleExpr, predicateExpr) with no
// grammar-authored dynamic precedence on either production. The locked C
// oracle always keeps the className/typeExpr reading; the declared-conflict
// election policy (grammars/runtime_profiles.go "ql",
// ConflictPolicyDeclaredReduceReduceHighestSymbol in conflict_policy.go)
// folds the row to that same reading before the GLR engine ever forks, so
// the raw (pre-result-compatibility) production tree already matches C
// natively.
func TestQLSignatureExprElectionRawCOracleParity(t *testing.T) {
	goLang := grammars.QlLanguage()
	tests := []struct {
		name   string
		source []byte
	}{
		{
			name: "bare_upper_id_signature",
			source: []byte("module Supply11 implements SupplyInt {\n" +
				"  int get() { result = 11 }\n" +
				"}\n"),
		},
		{
			name: "qualified_upper_id_signature",
			source: []byte("module M implements DataFlow::ConfigSig {\n" +
				"  predicate id() { any() }\n" +
				"}\n"),
		},
		{
			// Lower-case tails have no className/typeExpr alternative in C
			// either (className is an upper-id-only token): the moduleExpr
			// reading is the only parse, so the election must leave it
			// untouched rather than always preferring the higher symbol id.
			name: "lower_id_signature_stays_module_expr",
			source: []byte("module M implements dataflow {\n" +
				"  predicate id() { any() }\n" +
				"}\n"),
		},
		{
			// moduleInstantiation tails (`M::Inst<T>`) have no typeExpr
			// counterpart (typeExpr's name field accepts only className, never
			// moduleInstantiation), so this shape never reaches the declared
			// conflict at all; it must stay a moduleExpr election.
			name: "module_instantiation_tail_stays_module_expr",
			source: []byte("module M implements Base::Inst<int> {\n" +
				"  predicate id() { any() }\n" +
				"}\n"),
		},
	}

	cLang, err := COracleLanguage("ql")
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
			if len(mismatches) != 0 {
				t.Fatalf("raw Go and C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
