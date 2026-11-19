//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:73, 106-108, 136-137.
func TestFennelScannerCParityReproducers(t *testing.T) {
	sc := FennelExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"mask-lookahead", "\x6e\xe2\x80\xa8", 1, "000001100", "", false, 0, 0, 0, 2},
		{"ordinary-found", "\x60\x00", 0, "111100010", "", true, 74, 0, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// scanner.c:73 treats NUL inside a shebang as data, not EOF.
func TestFennelScannerCParityShebangNUL(t *testing.T) {
	checkScannerGroup1(t, FennelExternalScanner{}, scannerGroup1Case{"shebang-nul", "#!\x00a\n", 0, "000000010", "", true, 79, 0, 4, 5})
}
