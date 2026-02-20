package gotreesitter

import "testing"

func TestParseWithoutDFALexerReturnsEmptyTree(t *testing.T) {
	lang := &Language{Name: "no_dfa", InitialState: 1}
	parser := NewParser(lang)

	tree := parser.Parse([]byte("anything"))
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.RootNode() != nil {
		t.Fatal("expected nil root for language without DFA lexer")
	}
}

func TestParseIncrementalWithoutDFALexerReturnsEmptyTree(t *testing.T) {
	lang := &Language{Name: "no_dfa", InitialState: 1}
	parser := NewParser(lang)
	oldTree := NewTree(nil, []byte("old"), lang)

	tree := parser.ParseIncremental([]byte("new"), oldTree)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.RootNode() != nil {
		t.Fatal("expected nil root for language without DFA lexer")
	}
}
