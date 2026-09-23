package grammars

import (
	"strings"
	"testing"
)

func TestTsxJsxTextEquals(t *testing.T) {
	tests := []struct {
		name   string
		source string
		text   string
	}{
		{name: "spaced", source: "const a = <p>a = b</p>;\n", text: "a = b"},
		{name: "tight", source: "const a = <code>k=v</code>;\n", text: "k=v"},
		{name: "after_attribute", source: "const a = <p className = \"x\">a = b</p>;\n", text: "a = b"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "tsx", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("TSX parse is incomplete: %s", root.SExpr(lang))
			}
			textNode := pinFind(root, lang, "jsx_text")
			if textNode == nil {
				t.Fatalf("missing JSX text: %s", root.SExpr(lang))
			}
			start := strings.Index(test.source, test.text)
			if textNode.StartByte() != uint32(start) || textNode.EndByte() != uint32(start+len(test.text)) {
				t.Fatalf("JSX text range = [%d:%d], want [%d:%d]", textNode.StartByte(), textNode.EndByte(), start, start+len(test.text))
			}
		})
	}
}
