//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:24-34, 46-47.
func TestGitcommitScannerCParityReproducers(t *testing.T) {
	sc := GitcommitExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-lookahead", "\x5c\xc2\xa0", 1, "11", "\xff\xff\xff", false, 0, 0, 0, 4},
		{"ordinary-found", "\x65\xc2\xa0\x3a", 1, "10", "", true, 456, 1, 3, 4},
		{"ordinary-lookahead", "\x00\xc2\xa0", 1, "11", "", false, 0, 0, 0, 4},
		{"range-lookahead", "\x0a\x5c\xc2\xa0", 2, "10", "", false, 0, 0, 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// scanner.c:24-34 uses the C locale for control characters.
func TestGitcommitScannerCParityNonASCIIControl(t *testing.T) {
	checkScannerGroup1(t, GitcommitExternalScanner{}, scannerGroup1Case{"non-ascii-control", "\xc2\x85:", 0, "10", "", true, 456, 0, 2, 3})
}
