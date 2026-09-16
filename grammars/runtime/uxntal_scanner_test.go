//go:build !grammar_subset || grammar_subset_uxntal

package grammarruntime

import (
	"testing"
	_ "unsafe"

	"github.com/odvcencio/gotreesitter"
)

func TestUxntalScannerConsumesWhitespaceAndNestedComment(t *testing.T) {
	source := []byte("\n\t( outer (inner) tail )next")
	lexer := newUxntalExternalLexer(source, 0, 0, 0)
	if !((UxntalExternalScanner{}).Scan(nil, lexer, nil)) {
		t.Fatal("Scan returned false for a nested comment")
	}
	tok, ok := uxntalExternalLexerToken(lexer)
	if !ok {
		t.Fatal("Scan returned true without a token")
	}
	if got, want := tok.Symbol, uxntalSymComment; got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(len("\n\t( outer (inner) tail )")); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
	if got, want := lexer.Lookahead(), 'n'; got != want {
		t.Fatalf("lookahead = %q, want %q", got, want)
	}
}

func TestUxntalScannerRejectsUnterminatedComment(t *testing.T) {
	lexer := newUxntalExternalLexer([]byte(" (unterminated"), 0, 0, 0)
	if (UxntalExternalScanner{}).Scan(nil, lexer, []bool{true}) {
		t.Fatal("Scan returned true for an unterminated comment")
	}
	if tok, ok := uxntalExternalLexerToken(lexer); ok {
		t.Fatalf("Scan returned false but produced token %+v", tok)
	}
}

func TestParseUxntalConsecutiveCommentsStaysCleanAndSingleStacked(t *testing.T) {
	source := []byte("( first )\n( second ( nested ) )\n|00 @System &vector $2\n")
	lang := UxntalLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("root.HasError = true: %s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root.EndByte = %d, want %d", got, want)
	}
	if got := tree.ParseRuntime().MaxStacksSeen; got > 1 {
		t.Fatalf("MaxStacksSeen = %d, want <= 1", got)
	}
}

//go:linkname newUxntalExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newUxntalExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname uxntalExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func uxntalExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)
