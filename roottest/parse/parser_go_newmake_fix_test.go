package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestParseGoNewMakeLeadingCommentArgumentStillRetagged is the regression guard
// for the #384 review's finding #1: a LEADING COMMENT must not defeat the retag.
// Go attaches comments as named extras, so `new(/* c */ T)` has the comment as
// argument_list.NamedChild(0); the retag must skip it and still promote the
// first non-extra argument (the type) to type_identifier — matching C, which
// yields (argument_list (comment) (type_identifier)).
func TestParseGoNewMakeLeadingCommentArgumentStillRetagged(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"block-comment-before", "package p\n\nfunc f() {\n\ttype T int\n\t_ = new(/* c */ T)\n}\n"},
		{"line-comment-before-multiline", "package p\n\nfunc f() {\n\ttype T int\n\t_ = new(\n\t\t// leading\n\t\tT,\n\t)\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			assertNoErrorOrMissing(t, lang, root, tc.name)

			call := findNamedChild(lang, root, "call_expression")
			if call == nil {
				t.Fatalf("no call_expression; root=%s", root.SExpr(lang))
			}
			args := call.ChildByFieldName("arguments", lang)
			if args == nil {
				t.Fatalf("no arguments field; root=%s", root.SExpr(lang))
			}
			var first *gotreesitter.Node
			for i := 0; i < args.NamedChildCount(); i++ {
				if c := args.NamedChild(i); c != nil && c.Type(lang) != "comment" {
					first = c
					break
				}
			}
			if first == nil {
				t.Fatalf("no non-comment argument; root=%s", root.SExpr(lang))
			}
			if got := first.Type(lang); got != "type_identifier" {
				t.Fatalf("first non-comment argument Type() = %q, want type_identifier (leading comment must not defeat retag); root=%s", got, root.SExpr(lang))
			}
		})
	}
}

// TestParseGoNewMakeGenericCallArgumentNotRetagged is the regression guard for
// the #384 review's finding #2: a generic-instantiation shape (`new[T](x)`)
// parses with a type_arguments child and takes C's ordinary call branch, where
// the argument stays `identifier`. The retag must NOT fire when type_arguments
// is present (it would spuriously add a divergence). `new[int](x)` is invalid
// Go, so this only guards against a wrong retag, not full parse fidelity.
func TestParseGoNewMakeGenericCallArgumentNotRetagged(t *testing.T) {
	tree, lang := parseGo(t, "package p\n\nfunc f() {\n\tvar x int\n\t_ = new[int](x)\n}\n")
	root := tree.RootNode()

	var target *gotreesitter.Node
	for _, call := range findAllNodesOfType(lang, root, "call_expression") {
		for i := 0; i < call.NamedChildCount(); i++ {
			if c := call.NamedChild(i); c != nil && c.Type(lang) == "type_arguments" {
				target = call
				break
			}
		}
		if target != nil {
			break
		}
	}
	if target == nil {
		t.Skipf("no generic-instantiation call_expression parsed (grammar shape changed); root=%s", root.SExpr(lang))
	}
	args := target.ChildByFieldName("arguments", lang)
	if args == nil || args.NamedChildCount() == 0 {
		t.Fatalf("generic call has no argument; root=%s", root.SExpr(lang))
	}
	if got := args.NamedChild(0).Type(lang); got != "identifier" {
		t.Fatalf("generic call argument Type() = %q, want identifier (retag must not fire with type_arguments present); root=%s", got, root.SExpr(lang))
	}
}
