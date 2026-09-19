package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestJavaScriptReservedWordBindingAbsorbsTokensAsDirectChildren is a
// root-cause regression guard.
//
// `var if = 1;` uses the reserved word `if` where the grammar expects a
// binding identifier. pushOrExtendErrorNode's no-action fallback used to
// discard each absorbed real token and nest a second, childless ERROR inside
// the first ERROR, instead of keeping the absorbed tokens as the outer
// ERROR's own direct children. Tree-sitter C's oracle wraps each absorbed
// lookahead in an invisible error_repeat and hoists consecutive wraps' tokens
// flat into the final ERROR, so the C tree is:
//
//	(program (ERROR (var) (if) (=)) (expression_statement (number)))
//
// This must hold on both the default admission route and the production
// route.
func TestJavaScriptReservedWordBindingAbsorbsTokensAsDirectChildren(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	if lang == nil {
		t.Skip("javascript grammar not registered")
	}
	src := []byte("var if = 1;\n")

	for _, route := range []struct {
		name    string
		compact bool
	}{
		{"default_route", true},
		{"production_route", false},
	} {
		t.Run(route.name, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(route.compact)
			tree, err := p.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", src, err)
			}
			defer tree.Release()

			root := tree.RootNode()
			if root.ChildCount() == 0 {
				t.Fatalf("root has no children:\n%s", root.SExpr(lang))
			}
			errNode := root.Child(0)
			if !errNode.IsError() {
				t.Fatalf("root.Child(0) = %s, want an ERROR node:\n%s", errNode.Type(lang), root.SExpr(lang))
			}
			if got, want := errNode.StartByte(), uint32(0); got != want {
				t.Fatalf("ERROR start = %d, want %d:\n%s", got, want, root.SExpr(lang))
			}
			if got, want := errNode.EndByte(), uint32(8); got != want {
				t.Fatalf("ERROR end = %d, want %d:\n%s", got, want, root.SExpr(lang))
			}
			if got, want := errNode.ChildCount(), 3; got != want {
				t.Fatalf("ERROR child count = %d, want 3 (var, if, =):\n%s", got, root.SExpr(lang))
			}
			wantTexts := []string{"var", "if", "="}
			for i, want := range wantTexts {
				c := errNode.Child(i)
				if c.IsError() {
					t.Fatalf("ERROR child[%d] is itself an ERROR node (nested-ERROR regression):\n%s", i, root.SExpr(lang))
				}
				if got := c.Text(src); got != want {
					t.Fatalf("ERROR child[%d] text = %q, want %q:\n%s", i, got, want, root.SExpr(lang))
				}
			}
		})
	}
}

// TestJavaScriptRecoveryKeepsErrorWrappingSkippedBrace pins the second
// invariant Fix A must preserve: a stray closing brace on its own, with no
// absorbed real token beyond it, still gets an ERROR wrapper. The fix must
// not delete the ERROR entirely by pushing a bare, unwrapped token instead.
func TestJavaScriptRecoveryKeepsErrorWrappingSkippedBrace(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	if lang == nil {
		t.Skip("javascript grammar not registered")
	}
	src := []byte("let x = 1;\n}\nlet y = 2;\n")

	for _, route := range []struct {
		name    string
		compact bool
	}{
		{"default_route", true},
		{"production_route", false},
	} {
		t.Run(route.name, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(route.compact)
			tree, err := p.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", src, err)
			}
			defer tree.Release()

			root := tree.RootNode()
			if !findErrorNodeWrappingBrace(root, src) {
				t.Fatalf("expected an ERROR node wrapping the stray '}':\n%s", root.SExpr(lang))
			}
		})
	}
}

func findErrorNodeWrappingBrace(n *gts.Node, source []byte) bool {
	if n == nil {
		return false
	}
	if n.IsError() {
		for i := 0; i < n.ChildCount(); i++ {
			if n.Child(i).Text(source) == "}" {
				return true
			}
		}
	}
	for i := 0; i < n.ChildCount(); i++ {
		if findErrorNodeWrappingBrace(n.Child(i), source) {
			return true
		}
	}
	return false
}
