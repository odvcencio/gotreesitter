//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// QL declares conflicts: [[$.simpleId, $.className], ...]. An upper-case
// identifier in signature position (`implements Foo`, `implements M::Foo`)
// reduces the shared _upper_id token as either simpleId (building
// moduleExpr) or className (building typeExpr); both are live GLR
// alternatives of signatureExpr's choice(typeExpr, moduleExpr,
// predicateExpr) with no grammar-authored dynamic precedence on either
// production. The locked C oracle always keeps the className/typeExpr
// reading. The declared-conflict election policy
// (ConflictPolicyDeclaredReduceReduceHighestSymbol, wired for ql in
// runtime_profiles.go) folds this row before the GLR engine ever forks, so
// every route materializes the C shape natively; no post-parse rewrite
// remains.
var (
	qlBareSignatureFixture = []byte(
		"module Supply11 implements SupplyInt {\n" +
			"  int get() { result = 11 }\n" +
			"}",
	)
	qlQualifiedSignatureFixture = []byte(
		"module M implements DataFlow::ConfigSig {\n" +
			"  predicate id() { any() }\n" +
			"}",
	)
	qlLowerIDSignatureFixture = []byte(
		"module M implements dataflow {\n" +
			"  predicate id() { any() }\n" +
			"}",
	)
	qlModuleInstantiationTailFixture = []byte(
		"module M implements Base::Inst<int> {\n" +
			"  predicate id() { any() }\n" +
			"}",
	)
)

func TestQLSignatureExprElectionNeedsNoResultCompatibility(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := QlLanguage()

	for _, fixture := range [][]byte{qlBareSignatureFixture, qlQualifiedSignatureFixture} {
		tree, err := gotreesitter.NewParser(language).ParseNoResultCompatibilityBenchmarkOnly(fixture)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tree.Release)
		assertNoNormalizationPasses(t, tree)
		assertQLSignatureExprIsTypeExprClassName(t, "compatibility-free", tree.RootNode(), language, fixture)

		production, err := gotreesitter.NewParser(language).Parse(fixture)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(production.Release)
		if runtime := production.ParseRuntime(); runtime.NormalizationNodesRewritten != 0 {
			t.Fatalf("production normalization rewrote %d nodes", runtime.NormalizationNodesRewritten)
		}
		assertQLSignatureExprIsTypeExprClassName(t, "production", production.RootNode(), language, fixture)
	}
}

func TestQLSignatureExprElectionRoutesMatch(t *testing.T) {
	language := QlLanguage()

	tests := []struct {
		name    string
		fixture []byte
		assert  func(t *testing.T, route string, root *gotreesitter.Node, language *gotreesitter.Language, source []byte)
	}{
		{"bare_upper_id_signature", qlBareSignatureFixture, assertQLSignatureExprIsTypeExprClassName},
		{"qualified_upper_id_signature", qlQualifiedSignatureFixture, assertQLSignatureExprIsTypeExprClassName},
		{"lower_id_signature_stays_module_expr", qlLowerIDSignatureFixture, assertQLSignatureExprIsModuleExprSimpleId},
		{"module_instantiation_tail_stays_module_expr", qlModuleInstantiationTailFixture, assertQLSignatureExprIsModuleExprSimpleId},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipts := retiredDispatchRouteReceipts(t, language, test.fixture)
			for _, receipt := range receipts {
				source := test.fixture
				if receipt.name != "incremental" {
					// retiredDispatchRouteReceipts appends a trailing newline
					// to the production/compact/forest sources but reparses
					// the incremental route from the original fixture length
					// via an edit; assert against the exact bytes each tree
					// actually covers.
					source = append(append([]byte(nil), test.fixture...), '\n')
				}
				test.assert(t, receipt.name, receipt.tree.RootNode(), language, source)
			}
		})
	}
}

// assertQLSignatureExprIsTypeExprClassName locates the signatureExpr node
// and requires the C-native typeExpr(className) reading.
func assertQLSignatureExprIsTypeExprClassName(
	t *testing.T,
	route string,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	if root == nil || root.HasError() {
		t.Fatalf("%s root = %v", route, root)
	}
	sig := findRecoveryActionMaterializationNode(root, language, "signatureExpr")
	if sig == nil || sig.ChildCount() != 1 {
		t.Fatalf("%s signatureExpr = %v, want exactly one child: %s", route, sig, root.SExpr(language))
	}
	expr := sig.Child(0)
	if expr.Type(language) != "typeExpr" {
		t.Fatalf("%s signatureExpr child = %q, want %q: %s", route, expr.Type(language), "typeExpr", root.SExpr(language))
	}
	nameChild := expr.Child(expr.ChildCount() - 1)
	if nameChild.Type(language) != "className" {
		t.Fatalf("%s typeExpr trailing child = %q, want %q: %s", route, nameChild.Type(language), "className", root.SExpr(language))
	}
	_ = source
}

// assertQLSignatureExprIsModuleExprSimpleId locates the signatureExpr node
// and requires the moduleExpr reading, matching C on shapes with no
// typeExpr alternative (lower-case tails, moduleInstantiation tails).
func assertQLSignatureExprIsModuleExprSimpleId(
	t *testing.T,
	route string,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	if root == nil || root.HasError() {
		t.Fatalf("%s root = %v", route, root)
	}
	sig := findRecoveryActionMaterializationNode(root, language, "signatureExpr")
	if sig == nil || sig.ChildCount() != 1 {
		t.Fatalf("%s signatureExpr = %v, want exactly one child: %s", route, sig, root.SExpr(language))
	}
	expr := sig.Child(0)
	if expr.Type(language) != "moduleExpr" {
		t.Fatalf("%s signatureExpr child = %q, want %q: %s", route, expr.Type(language), "moduleExpr", root.SExpr(language))
	}
	_ = source
}
