//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:29-32, 87-98.
func TestNushellScannerCParityReproducers(t *testing.T) {
	sc := NushellExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"nul-content", "\x00#", 0, "010", "\x01", true, 270, 0, 0, 3},
		{"ordinary-lookahead", "\x7b\xc2\xa0", 1, "111", "", false, 0, 0, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// The parser can bind the external symbols in a different order from the
// scanner's enum; scanner.c:122 uses semantic token indexes.
func TestNushellScannerCParityReorderedSymbols(t *testing.T) {
	sc := NushellExternalScanner{externalToToken: []int{1, 0, 2}}
	checkScannerGroup1(t, sc, scannerGroup1Case{"reordered", "r#'", 0, "010", "", true, 269, 0, 3, 4})
}
