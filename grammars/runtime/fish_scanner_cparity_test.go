//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:24-30, 48-73.
func TestFishScannerCParityReproducers(t *testing.T) {
	sc := FishExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-found", "\x29\xc2\xa0", 1, "0110", "\xd8\xa8\xa7\x55\x22\xba\x98\xf8\x5a\x7a\xb2\x4e\xd8\xc1\xe2\x65\xfa", true, 54, 1, 1, 2},
		{"mask-found", "\x20\xe2\x80\xa8", 1, "1100", "", true, 53, 1, 1, 2},
		{"mask-lookahead", "\x0a\xc2\xa0", 1, "0011", "", false, 0, 0, 0, 2},
		{"ordinary-end", "\x29\xc2\xa0\xff", 1, "1111", "", true, 53, 1, 1, 2},
		{"ordinary-found", "\x09\xc2\xa0", 1, "0100", "", true, 54, 1, 1, 2},
		{"ordinary-lookahead", "\x29\xc2\xa0", 1, "1111", "", true, 53, 1, 1, 2},
		{"ordinary-point", "\x29\xc2\xa0\xff", 1, "1111", "", true, 53, 1, 1, 2},
		{"ordinary-start", "\x29\xc2\xa0\xff", 1, "1111", "", true, 53, 1, 1, 2},
		{"range-end", "\x0a\xc3\x29\xc2\xa0\xff", 3, "0101", "", true, 54, 3, 3, 4},
		{"range-found", "\x00\x0a\x09\xc2\xa0\x09\xf0\x9f", 3, "1000", "", true, 53, 3, 3, 4},
		{"range-lookahead", "\x0a\xc3\x29\xc2\xa0\xff", 3, "0011", "", false, 0, 0, 0, 4},
		{"range-point", "\x0a\xc3\x29\xc2\xa0\xff", 3, "0101", "", true, 54, 3, 3, 4},
		{"range-start", "\x0a\xc3\x29\xc2\xa0\xff", 3, "0101", "", true, 54, 3, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
