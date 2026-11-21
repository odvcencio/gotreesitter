//go:build !grammar_subset || grammar_subset_kconfig

package grammarruntime

import "testing"

// Reproducers from the pinned C scanner (amaanq/tree-sitter-kconfig 9ac99fe4,
// src/scanner.c). The expected results follow the C code, not the old port.
func TestKconfigScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src  string
		found      bool
		start, end uint32
	}
	for _, c := range []tc{
		// :24 iswspace is ASCII only in the C locale: U+00A0 is not white space, so it
		// is not skipped and the token starts at byte 0.
		{"nbsp-is-not-space", "\u00a0ab", true, 0, 4},
		// :24 the same for U+0085 (next line) and U+2003 (em space).
		{"nel-is-not-space", "\u0085ab", true, 0, 4},
		{"em-space-is-not-space", "\u2003ab", true, 0, 5},
		// :24 an ASCII vertical tab and form feed are white space, and are skipped.
		{"vt-ff-are-space", "\v\fab", true, 2, 4},
		// :41 only lexer->eof ends a line; a NUL byte inside the input is text.
		{"nul-is-text", "a\x00b\nc", true, 0, 5},
		{"nul-first-is-text", "\x00a", true, 0, 2},
		// :53 white space after a line is the ASCII class: the line feed is consumed and the
		// no-break space is not, so the token ends at byte 3 (the old port went on to byte 5).
		{"nbsp-is-not-indent", "\ta\n\u00a0b", true, 1, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			lexer := newPortParityLexer([]byte(c.src), 0, 0, 0)
			portInstallObserver(lexer)
			ok := KconfigExternalScanner{}.Scan(nil, lexer, []bool{true})
			r := portObserve(lexer, ok)
			if r.ok != c.found || (c.found && (r.start != c.start || r.end != c.end)) {
				t.Errorf("got found=%v start=%d end=%d; want found=%v start=%d end=%d", r.ok, r.start, r.end, c.found, c.start, c.end)
			}
		})
	}
}
