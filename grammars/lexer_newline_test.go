//go:build !grammar_subset

package grammars

import (
	"slices"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func tokenNames(tokens []gotreesitter.Token, lang *gotreesitter.Language) []string {
	var names []string
	for _, tok := range tokens {
		if tok.Symbol == 0 {
			break
		}
		if int(tok.Symbol) < len(lang.SymbolNames) {
			names = append(names, lang.SymbolNames[tok.Symbol])
		}
	}
	return names
}

func TestCNewlinePreprocToken(t *testing.T) {
	lang := CLanguage()
	src := []byte("#include <stdio.h>\nint x;\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource: %v", err)
	}
	tokens := collectAllTokens(ts, 100)
	names := tokenNames(tokens, lang)

	if !slices.Contains(names, "preproc_include_token2") {
		t.Errorf("expected preproc_include_token2 for preprocessor newline, got tokens: %v", names)
	}
}

func TestYAMLNewlineBlTokens(t *testing.T) {
	// YAML now uses an ExternalScanner (not TokenSource), so we test via
	// the parser's DFA+ExternalScanner path.
	lang := YamlLanguage()
	src := []byte("key: value\nother: data\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil tree")
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Errorf("YAML parse produced errors")
	}
}

func TestAuthzedNewlineTokens(t *testing.T) {
	lang := AuthzedLanguage()
	src := []byte("definition user {}\ndefinition doc {}\n")
	ts, err := NewAuthzedTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewAuthzedTokenSource: %v", err)
	}
	tokens := collectAllTokens(ts, 100)
	names := tokenNames(tokens, lang)

	if !slices.Contains(names, "\n") {
		t.Errorf("expected newline tokens for Authzed, got tokens: %v", names)
	}
}

func TestGoAutoSemicolon(t *testing.T) {
	lang := GoLanguage()
	src := []byte("package main\n\nfunc main() {\n\tx := 1\n}\n")
	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewGoTokenSource: %v", err)
	}
	tokens := collectAllTokens(ts, 100)

	hasSemicolon := false
	for _, tok := range tokens {
		if tok.Symbol == 0 {
			break
		}
		name := lang.SymbolNames[tok.Symbol]
		if name == ";" || name == "source_file_token1" {
			hasSemicolon = true
			break
		}
	}
	if !hasSemicolon {
		t.Error("expected auto-semicolon tokens for Go newlines")
	}
}

func TestJavaNoNewlineTokens(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("class Foo {\n  int x = 1;\n}\n")
	ts := NewJavaTokenSourceOrEOF(src, lang)
	tokens := collectAllTokens(ts, 100)
	names := tokenNames(tokens, lang)

	for _, name := range names {
		if name == "\n" {
			t.Errorf("Java should not produce newline tokens, but found one")
			break
		}
	}
}

func TestLuaNoNewlineTokens(t *testing.T) {
	lang := LuaLanguage()
	src := []byte("local x = 1\nprint(x)\n")
	ts := NewLuaTokenSourceOrEOF(src, lang)
	tokens := collectAllTokens(ts, 100)
	names := tokenNames(tokens, lang)

	for _, name := range names {
		if name == "\n" {
			t.Errorf("Lua should not produce newline tokens, but found one")
			break
		}
	}
}
