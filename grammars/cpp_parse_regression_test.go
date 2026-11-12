package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestCppUserTypeTemplateArgumentInParameterListParsesWithoutError(t *testing.T) {
	src := []byte("#include <vector>\nstruct Rule {};\nstatic inline void f(std::vector<Rule> *v) {}\n")
	parser := ts.NewParser(CppLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if root.HasError() {
		t.Fatalf("unexpected parse errors: %s runtime=%s", root.SExpr(CppLanguage()), tree.ParseRuntime().Summary())
	}
}

// C++23 `if consteval` is not in this grammar revision, so the C runtime
// reports an error here, but its tree still holds every token of the input:
//
//	(translation_unit (function_definition (primitive_type) (function_declarator
//	  (identifier) (parameter_list)) (compound_statement (ERROR)
//	  (consteval_block_declaration (compound_statement)))))
//
// The recovered tree must likewise report the error and keep every token.
func TestCppIfConstevalKeepsRecoveredTokens(t *testing.T) {
	src := []byte("void f() { if consteval { } }")
	tree, err := ts.NewParser(CppLanguage()).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	covered := make([]bool, len(src))
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n.ChildCount() == 0 {
			for i := n.StartByte(); i < n.EndByte() && int(i) < len(src); i++ {
				covered[i] = true
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	for i := 0; i < root.ChildCount(); i++ {
		walk(root.Child(i))
	}
	for i, c := range src {
		if c != ' ' && !covered[i] {
			t.Errorf("byte %d (%q) is under no leaf: %s", i, c, root.SExpr(CppLanguage()))
			break
		}
	}
	if !root.HasError() {
		t.Errorf("HasError = false, want true as in the C runtime: %s", root.SExpr(CppLanguage()))
	}
}
