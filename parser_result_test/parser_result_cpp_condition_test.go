package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCPPWhileAssignmentConditionParsesAsExpression(t *testing.T) {
	const src = "int f() { while ((a = b)) {} }\n"

	tree, lang := parseByLanguageName(t, "cpp", src)
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cpp parse error: %s", root.SExpr(lang))
	}

	stmt := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "while_statement"
	})
	if stmt == nil {
		t.Fatalf("missing while_statement: %s", root.SExpr(lang))
	}

	cond := firstNode(stmt, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "condition_clause"
	})
	if cond == nil {
		t.Fatalf("missing condition_clause: %s", stmt.SExpr(lang))
	}

	assign := firstNode(cond, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "assignment_expression"
	})
	if assign == nil {
		t.Fatalf("condition_clause missing assignment_expression: %s", cond.SExpr(lang))
	}

	if bad := firstNode(cond, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "declaration"
	}); bad != nil {
		t.Fatalf("condition_clause still parsed as declaration: %s", cond.SExpr(lang))
	}
}
