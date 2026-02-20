package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewJSONTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewJSONTokenSource([]byte(`{"a":1}`), lang); err == nil {
		t.Fatal("expected error for language missing json token symbols")
	}
}

func TestNewJSONTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewJSONTokenSourceOrEOF([]byte(`{"a":1}`), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestJSONTokenSourceSplitsStringEscapes(t *testing.T) {
	lang := JsonLanguage()
	src := []byte(`{"a":"x\n\u0041"}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	var sawContent, sawEscape bool
	for i := 0; i < 64; i++ {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		typ := lang.SymbolNames[tok.Symbol]
		if typ == "string_content" {
			sawContent = true
		}
		if typ == "escape_sequence" {
			sawEscape = true
		}
	}

	if !sawContent {
		t.Fatal("expected at least one string_content token")
	}
	if !sawEscape {
		t.Fatal("expected at least one escape_sequence token")
	}
}

func TestJSONTokenSourceSkipToByte(t *testing.T) {
	lang := JsonLanguage()
	src := []byte(`{"a":1, "target": 2}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	target := uint32(8) // points near "target"
	tok := ts.SkipToByte(target)
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if tok.StartByte < target {
		t.Fatalf("token starts before target: got %d, target %d", tok.StartByte, target)
	}
}

func TestParseJSONWithTokenSource(t *testing.T) {
	lang := JsonLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte(`{"a":[1,true,null,false,"x\n"],"b":{"c":2}}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	tree := parser.ParseWithTokenSource(src, ts)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected json parse without syntax errors")
	}
}
