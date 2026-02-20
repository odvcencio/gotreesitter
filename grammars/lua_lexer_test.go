package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewLuaTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewLuaTokenSource([]byte("local x = 1\n"), lang); err == nil {
		t.Fatal("expected error for language missing lua token symbols")
	}
}

func TestNewLuaTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewLuaTokenSourceOrEOF([]byte("local x = 1\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestLuaTokenSourceSkipToByte(t *testing.T) {
	lang := LuaLanguage()
	src := []byte("local x = 1\nlocal y = 2\n")
	target := bytes.Index(src, []byte("y"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
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

func TestParseLuaWithTokenSource(t *testing.T) {
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("local x = 1\n")
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected lua parse without syntax errors")
	}
}
