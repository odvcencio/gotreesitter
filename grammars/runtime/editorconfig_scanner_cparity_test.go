//go:build !grammar_subset || grammar_subset_editorconfig

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, src/scanner.c (ValdezFOmar/tree-sitter-editorconfig
// 63f104da). The expected results follow the C code, not the old port.
func TestEditorconfigScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		start, end      uint32
	}
	for _, c := range []tc{
		// :49 the end-of-file token needs lexer->eof; a NUL byte is data, so no token is made.
		{"nul-is-not-eof", "\x00", "10", false, 0, 0, 0},
		{"nul-is-not-eof-mid", "a\x00b", "10", false, 0, 0, 0},
		// :49-54 a real end of input still gives the token.
		{"real-eof", "", "10", true, 23, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := EditorconfigExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			lexer := newPortParityLexer([]byte(c.src), 0, 0, 0)
			portInstallObserver(lexer)
			ok := sc.Scan(nil, lexer, valid)
			r := portObserve(lexer, ok)
			if r.ok != c.found || (c.found && (r.sym != c.symbol || r.start != c.start || r.end != c.end)) {
				t.Errorf("got found=%v symbol=%d start=%d end=%d; want found=%v symbol=%d start=%d end=%d", r.ok, r.sym, r.start, r.end, c.found, c.symbol, c.start, c.end)
			}
		})
	}
}
