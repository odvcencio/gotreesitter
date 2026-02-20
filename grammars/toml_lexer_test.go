package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewTomlTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewTomlTokenSource([]byte("a = 1\n"), lang); err == nil {
		t.Fatal("expected error for language missing toml token symbols")
	}
}

func TestNewTomlTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewTomlTokenSourceOrEOF([]byte("a = 1\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestTomlTokenSourceSkipToByte(t *testing.T) {
	lang := TomlLanguage()
	src := []byte("a = 1\nb = 2\n")
	target := bytes.Index(src, []byte("b"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewTomlTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "b" {
		t.Fatalf("expected token text %q, got %q", "b", tok.Text)
	}
}

func TestParseTomlWithTokenSourceReturnsTree(t *testing.T) {
	lang := TomlLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("a = 1\n")
	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewTomlTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
}
