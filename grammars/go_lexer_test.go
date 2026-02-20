package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/mane/gotreesitter"
)

func TestNewGoTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}

	if _, err := NewGoTokenSource([]byte("package main\n"), lang); err == nil {
		t.Fatal("expected error for language missing go token symbols")
	}
}

func TestNewGoTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}

	ts := NewGoTokenSourceOrEOF([]byte("package main\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestGoTokenSourceSkipToByteReseek(t *testing.T) {
	lang := GoLanguage()

	var b strings.Builder
	b.WriteString("package main\n\nfunc main() {\n")
	for i := 0; i < 900; i++ {
		b.WriteString("\tx := 1\n")
	}
	b.WriteString("\ttarget := 2\n")
	b.WriteString("}\n")
	src := []byte(b.String())

	targetOffset := strings.Index(b.String(), "target")
	if targetOffset < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewGoTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(targetOffset))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < targetOffset {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, targetOffset)
	}
	if tok.Text != "target" {
		t.Fatalf("expected identifier token text %q, got %q", "target", tok.Text)
	}
}
