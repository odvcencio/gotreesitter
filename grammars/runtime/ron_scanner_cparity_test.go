//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:22, 41, 86-99, 140-145.
func TestRonScannerCParityReproducers(t *testing.T) {
	sc := RonExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"non-ascii-digit", "\xd9\xa1.0", 0, "0010", "", false, 0, 0, 0, 1},
		{"non-ascii-letter", "1.\xc3\xa9", 0, "0010", "", true, 25, 0, 2, 3},
		{"damaged-lookahead", "\xc2\xa0", 0, "0011", "\xfe\x6c\xaa\xf7\xda\x14\xbb\x54\x9a\xf7\xd8\xb7\x26\x92\x1f\x7a\x6a\x02\x9c\xb6\x8d\xd9\x07\x4f\xf8\x2e\x6c\xfa\x7e\x9d\x02\x69\xb2\x10\x8d", false, 0, 0, 0, 1},
		{"mask-lookahead", "\x20\xc2\xa0", 1, "1010", "", false, 0, 0, 0, 2},
		{"ordinary-lookahead", "\x0a\xc2\xa0", 1, "0001", "", false, 0, 0, 0, 2},
		{"range-lookahead", "\x20\x0a\x20\x20\xc2\xa0", 4, "1111", "", false, 0, 0, 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
