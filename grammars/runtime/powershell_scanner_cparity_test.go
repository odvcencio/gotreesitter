//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:32-43.
func TestPowershellScannerCParityReproducers(t *testing.T) {
	sc := PowershellExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"ordinary-found", "\xc2\xa0", 0, "10", "", false, 0, 0, 0, 1},
		{"ordinary-lookahead", "\x0a\xe2\x80\xa8", 1, "11", "", false, 0, 0, 0, 2},
		{"range-found", "\xc3\x20\x0c\xe2\x80\xa8\x0d\x0a\xff", 2, "10", "", false, 0, 0, 0, 4},
		{"range-lookahead", "\xc3\x20\x0c\xe2\x80\xa8\x0d\x0a\xff", 2, "10", "", false, 0, 0, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
