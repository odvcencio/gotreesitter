//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:20-24.
func TestTablegenScannerCParityReproducers(t *testing.T) {
	sc := TablegenExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"ordinary-lookahead", "\x74\xc2\xa0", 1, "1", "", false, 0, 0, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
