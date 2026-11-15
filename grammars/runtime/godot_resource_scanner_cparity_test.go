//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproducers and character-class behavior from the pinned C scanner, src/scanner.c:13-21.
func TestGodotResourceScannerCParityReproducers(t *testing.T) {
	sc := GodotResourceExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"ordinary-lookahead", "\xc2\xa0", 0, "1", "", false, 0, 0, 0, 1},
		{"range-lookahead", "\x0a\x0a\xc2\xa0\x20", 0, "1", "", false, 0, 0, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
