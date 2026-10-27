package scannercert

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// The root exposes no constructor for ExternalLexer and no reader for its
// scan result. The certifier reaches both through go:linkname, which the
// linker does not type-check. These tests fail loudly if the root changes the
// linked signatures or the lexer fields the recorder reads.
func TestLexerLinkContract(t *testing.T) {
	if _, err := lexerLayout(); err != nil {
		t.Fatal(err)
	}
	src := []byte("ab\ncde")
	lx := newLexer(src, 3, 1, 0)
	source, pos, row, column := lexerOrigin(lx)
	if string(source) != string(src) || pos != 3 || row != 1 || column != 0 {
		t.Fatalf("origin = %q %d %d %d, want %q 3 1 0", source, pos, row, column, src)
	}
	if _, ok := lexerToken(lx); ok {
		t.Fatal("a fresh lexer reports a result symbol")
	}
	lx.Advance(false)
	lx.Advance(false)
	lx.MarkEnd()
	lx.SetResultSymbol(gts.Symbol(7))
	tok, ok := lexerToken(lx)
	if !ok || tok.Symbol != 7 || tok.StartByte != 3 || tok.EndByte != 5 ||
		tok.EndPoint != (gts.Point{Row: 1, Column: 2}) {
		t.Fatalf("token = %+v ok=%v, want symbol 7 spanning bytes 3-5 ending at 1:2", tok, ok)
	}
	// Reading the origin must not disturb the scan state.
	_, pos, row, column = lexerOrigin(lx)
	if pos != 5 || row != 1 || column != 2 {
		t.Fatalf("cursor = %d %d %d, want 5 1 2", pos, row, column)
	}
}
