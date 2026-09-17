package grammarruntime

import (
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCooklangExternalScannerEmitsZeroWidthNewline(t *testing.T) {
	src := []byte("@a{}\n@b{}\n")
	newlineAt := uint32(4)

	lexer := newCooklangExternalLexer(src, int(newlineAt), 0, newlineAt)
	validSymbols := []bool{true}
	scanner := CooklangExternalScanner{}
	if !scanner.Scan(nil, lexer, validSymbols) {
		t.Fatal("Scan returned false for valid newline")
	}
	tok, ok := cooklangExternalLexerToken(lexer)
	if !ok {
		t.Fatal("scanner did not produce a token")
	}
	if got, want := tok.Symbol, cooklangSymNewline; got != want {
		t.Fatalf("token Symbol = %d, want %d", got, want)
	}
	if got, want := tok.StartByte, newlineAt; got != want {
		t.Fatalf("_newline StartByte = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, newlineAt; got != want {
		t.Fatalf("_newline EndByte = %d, want zero-width at %d", got, want)
	}
}

func TestCooklangParseProgressesAfterZeroWidthNewline(t *testing.T) {
	src := []byte("@a{}\n@b{}\n")
	lang := CooklangLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if got, min := root.EndByte(), uint32(5); got <= min {
		t.Fatalf("root EndByte = %d, want parse to progress beyond newline byte %d; tree=%s", got, min-1, root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("root NamedChildCount = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
}

//go:linkname newCooklangExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newCooklangExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname cooklangExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func cooklangExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)
