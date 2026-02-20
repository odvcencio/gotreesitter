package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewHTMLTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewHTMLTokenSource([]byte("<html></html>\n"), lang); err == nil {
		t.Fatal("expected error for language missing html token symbols")
	}
}

func TestNewHTMLTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewHTMLTokenSourceOrEOF([]byte("<html></html>\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestHTMLTokenSourceSkipToByte(t *testing.T) {
	lang := HtmlLanguage()
	src := []byte("<html><body>Hello</body></html>\n")
	target := bytes.Index(src, []byte("body"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewHTMLTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewHTMLTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
}

func TestParseHTMLWithTokenSource(t *testing.T) {
	lang := HtmlLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("<html><body>Hello</body></html>\n")
	ts, err := NewHTMLTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewHTMLTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected html parse without syntax errors")
	}
}
