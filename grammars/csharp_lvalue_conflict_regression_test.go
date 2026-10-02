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

// TestCSharpCollectionExpressionTrailingCommaMatchesLockedCShape keeps the
// trailing-comma witness of the conflict-order defect. The certified physical
// version order retains C's collection-expression arm on a precedence tie.
// The cgo harness also checks this source's full tree and flags against C.
func TestCSharpCollectionExpressionTrailingCommaMatchesLockedCShape(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	const src = "var x = [ y, ];\n"
	const cShape = "(compilation_unit (global_statement (local_declaration_statement " +
		"(variable_declaration (implicit_type) (variable_declarator (identifier) " +
		"(collection_expression (collection_element (expression_element (identifier)))))))))"

	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	got := root.SExpr(lang)
	if got != cShape {
		t.Fatalf("collection-expression tree\n got: %s\nwant locked C: %s", got, cShape)
	}
}
