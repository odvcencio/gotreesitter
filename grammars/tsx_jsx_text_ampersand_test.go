package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestTsxJsxTextAmpersand covers gotreesitter issue #1242: a bare '&' in TSX
// JSX text used to end jsx_text and get re-lexed as the bitwise-and
// operator, leaving an ERROR node. Real JSX text allows a literal '&' (tsc
// accepts it); only '&' that starts a full html_character_reference (for
// example "&amp;") should stop the text node. This mirrors the upstream
// tree-sitter-javascript#366 report and the '=' fix already applied to
// tsx_scanner.go for the same issue.
func TestTsxJsxTextAmpersand(t *testing.T) {
	tests := []struct {
		name   string
		source string
		text   string
	}{
		{name: "spaced", source: "const a = <p>Org & Team</p>;\n", text: "Org & Team"},
		{name: "tight", source: "const a = <p>AT&T</p>;\n", text: "AT&T"},
		{name: "alone", source: "const a = <p>&</p>;\n", text: "&"},
		{name: "double_ampersand", source: "const a = <p>a && b</p>;\n", text: "a && b"},
		{name: "nbsp_like_but_invalid", source: "const a = <p>a&nbsp b</p>;\n", text: "a&nbsp b"},
		{name: "multiline", source: "const a = <p>Org &\n  Team</p>;\n", text: "Org &\n  Team"},
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
			if start < 0 {
				t.Fatalf("test text %q not found in source %q", test.text, test.source)
			}
			if textNode.StartByte() != uint32(start) || textNode.EndByte() != uint32(start+len(test.text)) {
				t.Fatalf("JSX text range = [%d:%d] %q, want [%d:%d] %q",
					textNode.StartByte(), textNode.EndByte(), test.source[textNode.StartByte():textNode.EndByte()],
					start, start+len(test.text), test.text)
			}
		})
	}
}

// collectByType returns every node of the given type in DFS order.
func collectByType(n *gotreesitter.Node, lang *gotreesitter.Language, typ string, out *[]*gotreesitter.Node) {
	if n == nil {
		return
	}
	if n.Type(lang) == typ {
		*out = append(*out, n)
	}
	for i := 0; i < n.ChildCount(); i++ {
		collectByType(n.Child(i), lang, typ, out)
	}
}

// TestTsxJsxTextAmpersandNextToExpression covers text containing a bare '&'
// immediately after a '{expr}' JSX expression: the text node must still
// resume cleanly past the expression boundary.
func TestTsxJsxTextAmpersandNextToExpression(t *testing.T) {
	source := "const a = <p>Count: {n} & more</p>;\n"
	tree, lang := pinParse(t, "tsx", source)
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() || root.EndByte() != uint32(len(source)) {
		t.Fatalf("TSX parse is incomplete: %s", root.SExpr(lang))
	}
	var texts []*gotreesitter.Node
	collectByType(root, lang, "jsx_text", &texts)
	if len(texts) != 2 {
		t.Fatalf("want 2 jsx_text nodes around the expression, got %d: %s", len(texts), root.SExpr(lang))
	}
	want := []string{"Count: ", " & more"}
	for i, node := range texts {
		got := source[node.StartByte():node.EndByte()]
		if got != want[i] {
			t.Fatalf("jsx_text[%d] = %q, want %q", i, got, want[i])
		}
	}
	if pinFind(root, lang, "jsx_expression") == nil {
		t.Fatalf("missing jsx_expression: %s", root.SExpr(lang))
	}
}

// TestTsxJsxTextAmpersandEntity confirms a real HTML character reference
// still lexes as html_character_reference, not jsx_text, exactly as before
// this fix: only a bare '&' that cannot start a full reference is now
// treated as literal text.
func TestTsxJsxTextAmpersandEntity(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "named", source: "const a = <p>x &amp; y</p>;\n"},
		{name: "nbsp", source: "const a = <p>a&nbsp;b</p>;\n"},
		{name: "decimal", source: "const a = <p>a&#38;b</p>;\n"},
		{name: "hex", source: "const a = <p>a&#x26;b</p>;\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "tsx", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("TSX parse is incomplete: %s", root.SExpr(lang))
			}
			entity := pinFind(root, lang, "html_character_reference")
			if entity == nil {
				t.Fatalf("missing html_character_reference: %s", root.SExpr(lang))
			}
		})
	}
}

// TestTsxJsxTextRejectsAngleAndBrace locks the correct v0.53.0 behavior the
// issue asked to preserve: a bare '>' or '}' in JSX text still produces an
// ERROR, matching tsc (they require '&gt;' / '&rbrace;' or an expression
// escape). A stray '=>' in text also still errors, because it contains a
// literal '>'.
func TestTsxJsxTextRejectsAngleAndBrace(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "bare_gt", source: "const a = <p>a > b</p>;\n"},
		{name: "bare_rbrace", source: "const a = <p>a } b</p>;\n"},
		{name: "arrow", source: "const a = <p>x => y</p>;\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "tsx", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if !root.HasError() {
				t.Fatalf("expected an ERROR node, tsc also rejects this input: %s", root.SExpr(lang))
			}
		})
	}
}

// TestTsxAmpersandEqualsStillWorkAsOperators confirms the JSX-text fix does
// not loosen '&' or '=' lexing outside JSX text: they must still work as the
// bitwise-and operator, the assignment operator, and inside a JSX
// expression, where they are never JSX text.
func TestTsxAmpersandEqualsStillWorkAsOperators(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "bitwise_and_expr", source: "const c = a & b;\n"},
		{name: "assignment_expr", source: "let a;\na = b;\n"},
		{name: "jsx_expression_and", source: "const a = <p>{a & b}</p>;\n"},
		{name: "jsx_attribute_equals", source: "const a = <p className={x}>y</p>;\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "tsx", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("TSX parse is incomplete: %s", root.SExpr(lang))
			}
		})
	}
}
