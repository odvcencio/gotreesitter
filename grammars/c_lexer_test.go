package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/mane/gotreesitter"
)

func TestNewCTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewCTokenSource([]byte("int main(void) { return 0; }\n"), lang); err == nil {
		t.Fatal("expected error for language missing c token symbols")
	}
}

func TestNewCTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewCTokenSourceOrEOF([]byte("int main(void) { return 0; }\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestCTokenSourceSkipToByte(t *testing.T) {
	lang := CLanguage()
	src := []byte("int main(void) {\n  int x = 1;\n  return x;\n}\n")
	target := bytes.Index(src, []byte("return"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "return" {
		t.Fatalf("expected token text %q, got %q", "return", tok.Text)
	}
}

func TestParseCWithTokenSource(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("int main(void) { return 0; }\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected c parse without syntax errors")
	}
}
