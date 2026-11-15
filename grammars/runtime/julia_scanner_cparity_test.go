//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:147-180.
func TestJuliaScannerCParityReproducers(t *testing.T) {
	sc := JuliaExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-lookahead", "\x20\x5c", 1, "0010100110001100", "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", false, 0, 0, 0, 3},
		{"mask-lookahead", "\x0a\x24", 1, "0010000110001100", "", false, 0, 0, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
