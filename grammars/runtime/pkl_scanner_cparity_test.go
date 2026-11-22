//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers from the pinned C scanner, src/scanner.c (apple/tree-sitter-pkl a02fc36f).
// The expected results follow the C code: when several string tokens are valid, the first
// in the order sl, ml, sl1, ml1, sl2, ml2, ... decides (:259-292).
func TestPklScannerCParityReproducers(t *testing.T) {
	sc := PklExternalScanner{}
	ml1Sym := pklDefaultSymTable[pklTokMl1StringChars]
	ml2Sym := pklDefaultSymTable[pklTokMl2StringChars]
	for _, tc := range []scannerGroup1Case{
		// ml1 (index 8) comes before sl2 (index 2).
		{"ml1-before-sl2", "ab\"", 0, "00100000100000000", "", true, ml1Sym, 0, 3, 4},
		// ml2 (index 9) comes before sl3 (index 3).
		{"ml2-before-sl3", "ab\"", 0, "00010000010000000", "", true, ml2Sym, 0, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
