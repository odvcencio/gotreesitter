//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:39, 71-76, 134-153.
func TestLuaScannerCParityReproducers(t *testing.T) {
	sc := LuaExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-found", "\xc3", 0, "010001", "\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 68, 0, 0, 5},
		{"damaged-lookahead", "\xc3", 0, "010001", "\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 68, 0, 0, 5},
		{"mask-lookahead", "\x09\xe2\x80\xa8", 1, "000001", "", false, 0, 0, 0, 2},
		{"ordinary-lookahead", "\xe2\x80\xa8", 0, "100000", "", false, 0, 0, 0, 1},
		{"range-lookahead", "\x20\x0a\xc2\xa0\x09", 2, "100000", "", false, 0, 0, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
