//go:build !grammar_subset

package grammarruntime

import "testing"

// C src/scanner.c:493-510, 548-572 carries the brace count across handlers.
func TestCSharpScannerCParityBraceCarry(t *testing.T) {
	checkScannerGroup1(t, CSharpExternalScanner{}, scannerGroup1Case{
		"brace-carry", "{{{{{x", 0, "0000001010000", "\x00\x01\x02\x03\x03\x04", true, 214, 0, 6, 7,
	})
}

// Pinned C scanner src/scanner.c:225,350,409,522.
func TestCSharpScannerCParityReproducers(t *testing.T) {
	sc := CSharpExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-point", "\x0a\xc2\xa0", 1, "0001011011111", "\x02\x01\x00\x02\x03\x02", true, 217, 1, 3, 4},
		{"damaged-start", "\x0a\xc2\xa0", 1, "0001011011111", "\x02\x01\x00\x02\x03\x02", true, 217, 1, 3, 4},
		{"mask-end", "\x20\xe2\x80\xa8", 1, "1010001111001", "", true, 206, 1, 1, 2},
		{"mask-lookahead", "\xe2\x80\xa8", 0, "0110101100100", "", false, 0, 0, 0, 1},
		{"mask-point", "\x20\xe2\x80\xa8", 1, "1010001111001", "", true, 206, 1, 1, 2},
		{"mask-start", "\x20\xe2\x80\xa8", 1, "1010001111001", "", true, 206, 1, 1, 2},
		{"ordinary-lookahead", "\x20\xe2\x80\xa8", 1, "1111111111111", "", false, 0, 0, 0, 2},
		{"range-lookahead", "\x20\x0a\xe2\x80\xa8\x20\x20\x0a\x20\x20\x20\x20\x0c\xc3\x0d\x0a", 0, "0001000000000", "", false, 0, 0, 0, 3},
		// The range ends at byte 9; this lexer can represent that boundary
		// only by truncating the source there.
		{"range-point", "\x0a\x0a\x0a\x20\x20\x20\x20\xc2\xa0", 6, "0010111111010", "\x00\x00", true, 217, 7, 9, 10},
		{"range-start", "\x0a\x0a\x0a\x20\x20\x20\x20\xc2\xa0", 6, "0010111111010", "\x00\x00", true, 217, 7, 9, 10},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
