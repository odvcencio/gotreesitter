package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewJavaTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewJavaTokenSource([]byte("class Main { int x; }\n"), lang); err == nil {
		t.Fatal("expected error for language missing java token symbols")
	}
}

func TestNewJavaTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewJavaTokenSourceOrEOF([]byte("class Main { int x; }\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestJavaTokenSourceSkipToByte(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("class Main {\n  int x = 1;\n  int y = 2;\n}\n")
	target := bytes.Index(src, []byte("y"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "y" {
		t.Fatalf("expected token text %q, got %q", "y", tok.Text)
	}
}

func TestParseJavaWithTokenSource(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("class Main { int x; }\n")
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected java parse without syntax errors")
	}
}
