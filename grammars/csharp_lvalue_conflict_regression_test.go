package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestCSharpAddressOfLogicalAndKeepsBinaryExpression pins the C tree for
// `(a) && b` at tree-sitter-c-sharp 9150f7d5.
//
// Upstream issue 413 made the old grammar read `(a) && b` as
// `cast(a, &(&b))`, because the cast state accepted `&` as a unary start and
// the lexer split `&&` into two `&` tokens. Grammar 9150f7d5 moved address-of
// into `_address_of_expression` with an lvalue operand, so the cast arm dies
// at the statement end and the binary arm survives. The C oracle at that
// commit returns:
//
//	(binary_expression (parenthesized_expression (identifier)) (identifier))
//
// Production GLR reproduces that tree directly. A post-parse rewrite used to
// re-create the issue-413 shape (normalizeCSharpDereferenceLogicalAndCasts,
// removed with this test), which turned a C-exact parse into a divergence.
// The cgo oracle test TestCSharpGrammargenCGORegressionCases owns the live
// comparison; this host test keeps the shape pinned without cgo.
func TestCSharpAddressOfLogicalAndKeepsBinaryExpression(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	const src = "bool c = (a) && b;\n"
	const want = "(compilation_unit (global_statement (local_declaration_statement " +
		"(variable_declaration (predefined_type) (variable_declarator (identifier) " +
		"(binary_expression (parenthesized_expression (identifier)) (identifier)))))))"

	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if got := root.SExpr(lang); got != want {
		t.Fatalf("S-expression mismatch for %q\n got: %s\nwant: %s", src, got, want)
	}

	binary := findFirstNamedDescendantWhere(root, lang, "binary_expression", func(*gotreesitter.Node) bool { return true })
	if binary == nil {
		t.Fatal("no binary_expression node")
	}
	// The operator stays one two-byte "&&" token. Two one-byte "&" leaves are
	// the issue-413 shape and must not come back.
	var operator *gotreesitter.Node
	for i := 0; i < binary.ChildCount(); i++ {
		child := binary.Child(i)
		if child != nil && !child.IsNamed() {
			operator = child
			break
		}
	}
	if operator == nil {
		t.Fatal("binary_expression has no anonymous operator child")
	}
	if got := operator.Type(lang); got != "&&" {
		t.Fatalf("operator type = %q, want \"&&\"", got)
	}
	if got := operator.EndByte() - operator.StartByte(); got != 2 {
		t.Fatalf("operator span = %d bytes, want 2", got)
	}
}

// TestCSharpCollectionExpressionTrailingCommaKnownDivergence records the
// second witness of the same defect report. It is NOT fixed, so the test
// asserts today's Go shape and names the C shape it must become.
//
// The C oracle at tree-sitter-c-sharp 9150f7d5 returns
// `collection_expression(collection_element(expression_element(identifier)))`
// for `var x = [ y, ];`. Go returns
// `element_binding_expression(argument(identifier))`.
//
// Both engines build both derivations and both merge them at state 2101,
// byte 15. Both keep the incumbent version on a dynamic-precedence tie. The
// versions arrive in opposite order:
//
//   - C gives the CURRENT version the LAST action of a conflict cell and
//     appends the earlier actions as new, higher-indexed versions
//     (lib/src/parser.c ts_parser__advance, the
//     ts_stack_renumber_version(last_reduction_version, version) call).
//   - Go gives the current stack actions[0] and appends actions[1:] as forks
//     (parser.go, the conflict-fork block).
//
// So C's incumbent is the collection_expression arm and Go's is the
// bracketed_argument_list arm. Reversing the Go order alone does not fix it:
// the boundary merge then declines, because the two stacks hold different
// physical forms (one flat, one packed) and the mixed flat/GSS receiver path
// is certified per grammar artifact (Language.CompactMixedGSSMergeCertified,
// today python only).
//
// The repair needs the merge-time-election lane, not a local patch. Update
// this test together with that lane.
func TestCSharpCollectionExpressionTrailingCommaKnownDivergence(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	const src = "var x = [ y, ];\n"
	const cShape = "(compilation_unit (global_statement (local_declaration_statement " +
		"(variable_declaration (implicit_type) (variable_declarator (identifier) " +
		"(collection_expression (collection_element (expression_element (identifier)))))))))"
	const goShape = "(compilation_unit (global_statement (local_declaration_statement " +
		"(variable_declaration (implicit_type) (variable_declarator (identifier) " +
		"(element_binding_expression (argument (identifier))))))))"

	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	got := root.SExpr(lang)
	if got == cShape {
		t.Fatalf("the known divergence is gone; replace this test with an equality assertion against:\n%s", cShape)
	}
	if got != goShape {
		t.Fatalf("known divergence changed shape\n got: %s\nwant: %s\n(C oracle: %s)", got, goShape, cShape)
	}
}
