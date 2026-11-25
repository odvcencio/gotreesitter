package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Known gap: pinned tree-sitter-javascript 58404d8c ends JSX text at a bare
// '&', and the certified core follows pinned C, so these inputs give pinned
// C's ERROR tree. The follow-up to #1770 patches the C oracle and the core the
// way #1709 did for TSX; then these cases get their clean expectation back.
const (
	jsxAmpersandGapText  = "(program (lexical_declaration (variable_declarator (identifier) (jsx_element (jsx_opening_element (identifier)) (jsx_text) (ERROR (identifier)) (jsx_closing_element (identifier))))))"
	jsxAmpersandGapAlone = "(program (lexical_declaration (variable_declarator (identifier) (jsx_element (jsx_opening_element (identifier)) (ERROR) (jsx_closing_element (identifier))))))"
)

// requireJsxAmpersandKnownGap asserts pinned C's tree for a bare '&' in JSX
// text: the S-expression, and an ERROR node that starts at the first '&'.
func requireJsxAmpersandKnownGap(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language, source, pinnedC string) {
	t.Helper()
	if !root.HasError() {
		t.Fatalf("%q parses without an ERROR node now: the JavaScript JSX text scanner patch (the follow-up to #1770) has landed, so restore this case's clean expectation: %s", source, root.SExpr(lang))
	}
	if got := root.SExpr(lang); got != pinnedC {
		t.Fatalf("known gap: tree = %s, want pinned C's tree %s", got, pinnedC)
	}
	errorNode := pinFind(root, lang, "ERROR")
	if at := strings.IndexByte(source, '&'); errorNode == nil || errorNode.StartByte() != uint32(at) {
		t.Fatalf("known gap: pinned C's ERROR node starts at the first '&' (byte %d): %s", at, root.SExpr(lang))
	}
}

// TestJavaScriptJsxTextAmpersandAndEquals covers gotreesitter issue #1242 for
// the javascript grammar: a bare '&' or '=' in JSX text produced an ERROR
// node for the same reason as tsx (see tsx_jsx_text_ampersand_test.go and
// tsx_jsx_text_equals_test.go). This was not a regression for javascript --
// v0.20.2 already failed these inputs -- but it is the same mechanism, so it
// gets the same fix.
func TestJavaScriptJsxTextAmpersandAndEquals(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		text    string
		pinnedC string // set while the bare-'&' known gap lasts
	}{
		{name: "ampersand_spaced", source: "const a = <p>Org & Team</p>;\n", text: "Org & Team", pinnedC: jsxAmpersandGapText},
		{name: "ampersand_tight", source: "const a = <p>AT&T</p>;\n", text: "AT&T", pinnedC: jsxAmpersandGapText},
		{name: "ampersand_alone", source: "const a = <p>&</p>;\n", text: "&", pinnedC: jsxAmpersandGapAlone},
		{name: "equals_spaced", source: "const a = <p>a = b</p>;\n", text: "a = b"},
		{name: "equals_tight", source: "const a = <code>k=v</code>;\n", text: "k=v"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "javascript", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if test.pinnedC != "" {
				requireJsxAmpersandKnownGap(t, root, lang, test.source, test.pinnedC)
				return
			}
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("javascript parse is incomplete: %s", root.SExpr(lang))
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

// TestJavaScriptJsxTextEntityAndRejections confirms the javascript grammar
// keeps recognizing real HTML character references and keeps rejecting a
// bare '>' or '}' in JSX text, matching tsc and the tsx grammar.
func TestJavaScriptJsxTextEntityAndRejections(t *testing.T) {
	clean := []struct {
		name    string
		source  string
		pinnedC string // set while the bare-'&' known gap lasts
	}{
		{name: "amp_entity", source: "const a = <p>x &amp; y</p>;\n"},
		{name: "double_ampersand", source: "const a = <p>a && b</p>;\n", pinnedC: jsxAmpersandGapText},
	}
	for _, test := range clean {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "javascript", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if test.pinnedC != "" {
				requireJsxAmpersandKnownGap(t, root, lang, test.source, test.pinnedC)
				return
			}
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("javascript parse is incomplete: %s", root.SExpr(lang))
			}
		})
	}

	rejected := []struct {
		name   string
		source string
	}{
		{name: "bare_gt", source: "const a = <p>a > b</p>;\n"},
		{name: "bare_rbrace", source: "const a = <p>a } b</p>;\n"},
	}
	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "javascript", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if !root.HasError() {
				t.Fatalf("expected an ERROR node, tsc also rejects this input: %s", root.SExpr(lang))
			}
		})
	}
}

// TestJavaScriptAmpersandEqualsStillWorkAsOperators confirms the JSX-text fix
// does not loosen '&' or '=' lexing outside JSX text.
func TestJavaScriptAmpersandEqualsStillWorkAsOperators(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "bitwise_and_expr", source: "const c = a & b;\n"},
		{name: "assignment_expr", source: "let a;\na = b;\n"},
		{name: "jsx_expression_and", source: "const a = <p>{a & b}</p>;\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, lang := pinParse(t, "javascript", test.source)
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint32(len(test.source)) {
				t.Fatalf("javascript parse is incomplete: %s", root.SExpr(lang))
			}
		})
	}
}
