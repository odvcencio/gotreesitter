//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:30-35, 137-195, 219-231.
func TestOdinScannerCParityReproducers(t *testing.T) {
	sc := OdinExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-lookahead", "\x00\xc2\xa0", 1, "0100110", "\x68\x2d\x4c\x0e\x86", false, 0, 0, 0, 2},
		{"mask-lookahead", "\x0a\xc2\xa0", 1, "0000001", "", false, 0, 0, 0, 2},
		{"mask-point", "\x0a\xe2\x80\xa8", 0, "1101001", "", true, 123, 0, 1, 5},
		{"mask-start", "\x0a\xe2\x80\xa8", 0, "1101001", "", true, 123, 0, 1, 5},
		{"ordinary-lookahead", "\x00\xc2\xa0", 1, "1101111", "", false, 0, 0, 0, 2},
		{"ordinary-point", "\x0a\xe2\x80\xa8", 0, "1100110", "", true, 123, 0, 1, 5},
		{"ordinary-start", "\x0a\xe2\x80\xa8", 0, "1100110", "", true, 123, 0, 1, 5},
		{"range-lookahead", "\x00\x0a\x09\xc2\xa0\x09\xf0\x9f", 2, "1110100", "", false, 0, 0, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// scanner.c:180-204 truncates the lookahead to char before strcmp.
func TestOdinScannerCParityTruncatedNextWord(t *testing.T) {
	checkScannerGroup1(t, OdinExternalScanner{}, scannerGroup1Case{"truncated-next-word", "\n\u017b ", 0, "1000010", "", false, 0, 0, 0, 4})
}
