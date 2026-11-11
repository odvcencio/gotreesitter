package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// TestOCamlCollapsedNamedLeafChildrenNativeRetiredDispatchArm is the R2
// retirement receipt for dispatch.ocaml
// (https://github.com/odvcencio/gotreesitter/blob/9c955f3d29b126973da657124438f7c0aeddce5b/docs/root-normalization-retirement.md,
// testdata/result_compat_ownership_v1.json).
// normalizeOCamlCollapsedNamedLeafChildren (parser_result_ocaml.go, now
// deleted) restored a missing anonymous token child under childless
// `boolean`/`and_operator`/`or_operator` leaves. The same class of bug was
// already proven fixed natively for Elixir's `boolean` node by
// TestElixirBooleanKeepsTrueTokenChildViaEngine
// (elixir_boolean_parse_regression_test.go) once
// shouldKeepVisibleAnonymousTokenChild started keeping different-named
// single-token wrappers unconditionally; this test proves the same engine
// invariant also covers OCaml's three collapsed-leaf rules, matching the
// dispatcher census's zero-rewrite receipt over the real OCaml corpus.
func TestOCamlCollapsedNamedLeafChildrenNativeRetiredDispatchArm(t *testing.T) {
	lang := OcamlLanguage()

	cases := []struct {
		name     string
		src      string
		nodeType string
		token    string
	}{
		{"boolean_true", "let x = true\n", "boolean", "true"},
		{"boolean_false", "let x = false\n", "boolean", "false"},
		{"and_operator_ampersand", "let x = a & b\n", "and_operator", "&"},
		{"and_operator_double_ampersand", "let x = a && b\n", "and_operator", "&&"},
		{"or_operator_keyword", "let x = a or b\n", "or_operator", "or"},
		{"or_operator_double_bar", "let x = a || b\n", "or_operator", "||"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			tree, err := ts.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("expected clean parse: %s", root.SExpr(lang))
			}
			node := findFirstNamedDescendantWhere(root, lang, tc.nodeType, func(*ts.Node) bool { return true })
			if node == nil {
				t.Fatalf("missing %s node: %s", tc.nodeType, root.SExpr(lang))
			}
			if got, want := node.ChildCount(), 1; got != want {
				t.Fatalf("%s child count = %d, want %d (collapsed leaf, not restored natively): %s", tc.nodeType, got, want, root.SExpr(lang))
			}
			child := node.Child(0)
			if child == nil || child.Type(lang) != tc.token || child.IsNamed() {
				t.Fatalf("%s child = %v, want anonymous %q", tc.nodeType, child, tc.token)
			}
		})
	}
}

func TestOCamlCollapsedNamedLeafChildrenRetiredDispatchArmRoutes(t *testing.T) {
	lang := OcamlLanguage()
	receipts := retiredDispatchRouteReceipts(t, lang, []byte("let x = a && b"))
	for _, receipt := range receipts {
		node := findFirstNamedDescendantWhere(
			receipt.tree.RootNode(),
			lang,
			"and_operator",
			func(*ts.Node) bool { return true },
		)
		if node == nil {
			t.Fatalf("%s route is missing and_operator", receipt.name)
		}
		child := node.Child(0)
		if node.ChildCount() != 1 || child == nil || child.Type(lang) != "&&" || child.IsNamed() {
			t.Fatalf("%s route did not retain the anonymous && child", receipt.name)
		}
	}
}
