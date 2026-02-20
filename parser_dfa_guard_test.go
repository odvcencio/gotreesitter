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

func TestParseWithIncompatibleLanguageVersionReturnsEmptyTree(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)

	tree := parser.Parse([]byte("1+2"))
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.RootNode() != nil {
		t.Fatal("expected nil root for incompatible language version")
	}
}

func TestParseWithTokenSourceIncompatibleLanguageVersionReturnsEmptyTree(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)
	ts := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("1+2")),
		language:          lang,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tree := parser.ParseWithTokenSource([]byte("1+2"), ts)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.RootNode() != nil {
		t.Fatal("expected nil root for incompatible language version")
	}
}

func TestParseIncrementalWithIncompatibleLanguageVersionReturnsEmptyTree(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)
	oldTree := NewTree(nil, []byte("1+2"), lang)

	tree := parser.ParseIncremental([]byte("1+3"), oldTree)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.RootNode() != nil {
		t.Fatal("expected nil root for incompatible language version")
	}
}
