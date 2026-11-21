//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, src/scanner.c (hankthetank27/tree-sitter-liquid
// fa11c7ba). The expected results follow the C code, not the old port: is_next_and_advance
// (:39-43) consumes its character whether or not it matches, iswspace is ASCII-only in
// the C locale (:19-23), and the front matter loop tests lexer->eof, not a NUL byte (:97-104).
func TestLiquidScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		start, end      uint32
	}
	for _, c := range []tc{
		// :118-123 the '%' is consumed by the newline test, so "%}" right after '#' does not end the comment.
		{"inline-percent-first", "#%}x", "100000", false, 0, 0, 0},
		// :150-156 the character after '{' is consumed even when it is not '%'.
		{"paired-brace-brace", "{{%endcomment%}", "010000", false, 0, 0, 0},
		// :196-202 the character after the tag name is consumed even when it is not '%', so the "{" that
		// opens the next tag is lost and no closing tag is found.
		{"paired-skips-after-tag", "{%endcomment{%endcomment%}", "010000", false, 0, 0, 0},
		// :19-23 U+00A0 is not whitespace in the C locale.
		{"inline-after-nbsp", "\xc2\xa0# x\n", "100000", false, 0, 0, 0},
		// :97-104 a NUL byte inside the front matter does not end the input.
		{"front-matter-nul-line", "---\n\x00\n---\n", "000010", true, 102, 0, 10},
		// :87-93 the closing line advances at a NUL byte too.
		{"front-matter-nul-after-dashes", "---\n---\x00x", "000010", true, 102, 0, 8},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := LiquidExternalScanner{}
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
