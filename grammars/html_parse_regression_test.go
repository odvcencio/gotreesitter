package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func findHTMLElementStartingAt(root *gotreesitter.Node, lang *gotreesitter.Language, start uint32) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == "element" && root.StartByte() == start {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findHTMLElementStartingAt(root.Child(i), lang, start); found != nil {
			return found
		}
	}
	return nil
}

func TestParseHTMLDeeplyNestedCustomKeepsRecoveredInnerRange(t *testing.T) {
	lang := HtmlLanguage()
	src := []byte("<div>\n" + strings.Repeat("<abc>", 900) + "\n</div>\n")

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse() returned nil root")
	}
	root := tree.RootNode()
	if got, want := root.Type(lang), "document"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatal("root.HasError = true, want false")
	}

	outer := root.Child(0)
	if outer == nil || outer.ChildCount() < 3 {
		t.Fatalf("outer = %#v, childCount = %d, want >= 3", outer, outer.ChildCount())
	}
	inner := outer.Child(1)
	endTag := outer.Child(2)
	if inner == nil || endTag == nil || endTag.ChildCount() == 0 {
		t.Fatalf("unexpected outer children: inner=%#v endTag=%#v", inner, endTag)
	}
	closeTok := endTag.Child(0)
	if closeTok == nil {
		t.Fatal("endTag.Child(0) = nil")
	}
	if got, want := inner.EndByte(), closeTok.StartByte(); got != want {
		t.Fatalf("inner.EndByte = %d, want %d", got, want)
	}
	if got, want := inner.EndPoint(), closeTok.StartPoint(); got != want {
		t.Fatalf("inner.EndPoint = %#v, want %#v", got, want)
	}
}

func TestParseHTMLRecoveredRangeOnlyExtendsUnclosedChain(t *testing.T) {
	lang := HtmlLanguage()
	tests := []struct {
		name      string
		source    string
		element   string
		wantEndAt string
	}{
		{
			name:      "unclosed nested custom chain reaches enclosing close",
			source:    "<div><abc><def>\n</div>",
			element:   "<abc>",
			wantEndAt: "</div>",
		},
		{
			name:      "closed child keeps its own range before trailing trivia",
			source:    "<div><span>x</span>\n</div>",
			element:   "<span>",
			wantEndAt: "\n",
		},
		{
			name:      "closed last sibling keeps its own range before trailing trivia",
			source:    "<div><span>x</span>\n<em>y</em>\n</div>",
			element:   "<em>",
			wantEndAt: "\n</div>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			tree, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("Parse() returned nil root")
			}
			t.Cleanup(tree.Release)

			start := strings.Index(tc.source, tc.element)
			if start < 0 {
				t.Fatalf("source does not contain element marker %q", tc.element)
			}
			element := findHTMLElementStartingAt(tree.RootNode(), lang, uint32(start))
			if element == nil {
				t.Fatalf("element starting at %d not found: %s", start, tree.RootNode().SExpr(lang))
			}
			wantEnd := strings.Index(tc.source, tc.wantEndAt)
			if wantEnd < 0 {
				t.Fatalf("source does not contain end marker %q", tc.wantEndAt)
			}
			if got := element.EndByte(); got != uint32(wantEnd) {
				t.Fatalf("element %q EndByte() = %d, want %d; text=%q tree=%s", tc.element, got, wantEnd, element.Text(src), tree.RootNode().SExpr(lang))
			}
		})
	}
}
