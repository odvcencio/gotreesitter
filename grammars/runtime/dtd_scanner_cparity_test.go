//go:build !grammar_subset || grammar_subset_dtd

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, dtd/src/scanner.c with common/scanner.h
// (tree-sitter-grammars/tree-sitter-xml 5000ae8f). The expected results follow the C code,
// not the old port: iswalpha and iswalnum are ASCII-only in the C locale
// (common/scanner.h:37-45) and the loops test lexer->eof, not a NUL byte
// (common/scanner.h:109, 135).
func TestDtdScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		start, end      uint32
	}
	for _, c := range []tc{
		// :43-45 U+00E9 is not a letter in the C locale, so it cannot start a name.
		{"target-nonascii-start", "éa", "100", false, 0, 0, 0},
		// :37-39 U+00E9 does not continue a name: the target ends before it.
		{"target-nonascii-letter-ends-name", "abéc", "100", true, 58, 0, 2},
		// :37-39 an Arabic-indic digit is not a digit in the C locale.
		{"target-nonascii-digit-ends-name", "a١", "100", true, 58, 0, 1},
		// :109 a NUL byte inside the content does not end the loop.
		{"content-nul", "a\x00b?>\n", "010", true, 59, 0, 3},
		// :135 a NUL byte inside the comment does not end the loop.
		{"comment-nul", "<!--\x00-->", "001", true, 60, 0, 8},
		// :135-152 a real end of input inside a comment gives no token.
		{"comment-unterminated", "<!-- x", "001", false, 0, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := DtdExternalScanner{}
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
