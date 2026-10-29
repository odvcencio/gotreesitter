package grammars

import (
	"strings"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// TestHTMLNestedCustomTagReconstructionRetiredDispatchArm is the R2
// retirement receipt for dispatch.html
// (docs/root-normalization-retirement.md,
// testdata/result_compat_ownership_v1.json).
// normalizeHTMLRecoveredNestedCustomTags (parser_result_html.go, now
// deleted) rebuilt a flat ERROR root of bare unclosed start_tag siblings
// (produced when several custom elements were left open before a mismatched
// close tag) into a properly nested element chain. The reduce engine no
// longer produces that flat shape: a genuinely mismatched close tag now
// isolates its own localized ERROR/erroneous_end_tag sibling while the
// matched prefix is already nested natively, and an auto-closeable chain
// (the close tag matches an ancestor) now parses with no error at all. Both
// confirm the dispatcher census's zero-rewrite receipt over the real HTML
// corpus, including the deeply-nested-custom stress fixture.
func TestHTMLNestedCustomTagReconstructionRetiredDispatchArm(t *testing.T) {
	lang := HtmlLanguage()

	t.Run("mismatched_close_isolates_localized_error_and_nests_matched_prefix", func(t *testing.T) {
		src := []byte("<my-a><my-b><my-c>text</my-x>\n")
		tree, err := ts.NewParser(lang).Parse(src)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		root := tree.RootNode()
		if got, want := root.Type(lang), "document"; got != want {
			t.Fatalf("root.Type = %q, want %q (no flat ERROR root of bare start_tag siblings): %s", got, want, root.SExpr(lang))
		}
		if !root.HasError() {
			t.Fatal("expected the mismatched close tag to still report an error somewhere in the tree")
		}
		outer := root.Child(0)
		if outer == nil || outer.Type(lang) != "element" {
			t.Fatalf("root.Child(0) = %v, want element (matched prefix already nested natively): %s", outer, root.SExpr(lang))
		}
		inner := findFirstNamedDescendantWhere(outer, lang, "element", func(n *ts.Node) bool {
			return n != outer
		})
		if inner == nil {
			t.Fatalf("expected nested element chain under the matched prefix: %s", root.SExpr(lang))
		}
	})

	t.Run("autoclosing_unclosed_custom_chain_parses_without_error", func(t *testing.T) {
		src := []byte("<div><xyz><xyz><xyz>text</div>\n")
		tree, err := ts.NewParser(lang).Parse(src)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		root := tree.RootNode()
		if root.HasError() {
			t.Fatalf("expected auto-closing nested custom chain to parse cleanly: %s", root.SExpr(lang))
		}
		if got, want := root.Type(lang), "document"; got != want {
			t.Fatalf("root.Type = %q, want %q", got, want)
		}
	})
}

func TestHTMLNestedCustomTagReconstructionRetiredDispatchArmRoutes(t *testing.T) {
	lang := HtmlLanguage()
	const source = "<div><xyz><xyz>text</div>"
	receipts := retiredDispatchRouteReceipts(t, lang, []byte(source))
	firstStart := strings.Index(source, "<xyz>")
	secondStart := strings.LastIndex(source, "<xyz>")
	closeStart := strings.Index(source, "</div>")
	if firstStart < 0 || secondStart < 0 || closeStart < 0 {
		t.Fatal("test source lacks a required HTML marker")
	}
	wantEndPoint := pointAtByte([]byte(source+"\n"), closeStart)

	for _, receipt := range receipts {
		root := receipt.tree.RootNode()
		if root.HasError() || root.Type(lang) != "document" {
			t.Fatalf("%s route returned an invalid document: %s", receipt.name, root.SExpr(lang))
		}
		outer := findFirstNamedDescendantWhere(root, lang, "element", func(*ts.Node) bool { return true })
		if outer == nil {
			t.Fatalf("%s route is missing the outer element: %s", receipt.name, root.SExpr(lang))
		}
		inner := findFirstNamedDescendantWhere(outer, lang, "element", func(node *ts.Node) bool {
			return node != outer
		})
		if inner == nil {
			t.Fatalf("%s route is missing the nested element: %s", receipt.name, root.SExpr(lang))
		}
		for _, start := range []int{firstStart, secondStart} {
			element := findHTMLElementStartingAt(root, lang, uint32(start))
			if element == nil {
				t.Fatalf("%s route is missing the element at %d: %s", receipt.name, start, root.SExpr(lang))
			}
			if got := element.EndByte(); got != uint32(closeStart) {
				t.Fatalf("%s route element at %d ends at %d, want %d", receipt.name, start, got, closeStart)
			}
			if got := element.EndPoint(); got != wantEndPoint {
				t.Fatalf("%s route element at %d ends at %#v, want %#v", receipt.name, start, got, wantEndPoint)
			}
		}
		if receipt.name == "incremental" {
			profile := receipt.incrementalProfile
			if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
			}
		}
	}
}
