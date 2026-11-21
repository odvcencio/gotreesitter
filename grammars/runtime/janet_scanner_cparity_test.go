//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers from the pinned C scanner, src/scanner.c (sogaiu/tree-sitter-janet-simple
// d1831869). The expected results follow the C code: iswspace is ASCII-only in the C
// locale (:49-52), the end of the input is lexer->eof, not a NUL byte (:75-77, :96-98),
// and the whitespace is skipped before the valid symbols are looked at (:49-55).
func TestJanetScannerCParityReproducers(t *testing.T) {
	sc := JanetExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"nbsp-is-not-space", "\xc2\xa0`a`", 0, "11", "", false, 0, 0, 0, 1},
		{"nul-after-backtick", "`\x00`", 0, "01", "", true, 27, 0, 3, 4},
		{"nul-inside-string", "`a\x00`", 0, "01", "", true, 27, 0, 4, 5},
		{"space-skipped-without-valid-symbols", " \n`a`", 0, "00", "", false, 0, 0, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
