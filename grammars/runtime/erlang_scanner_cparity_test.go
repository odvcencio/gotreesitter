//go:build !grammar_subset

package grammarruntime

import "testing"

// Pinned C scanner src/scanner.c:62-79, 128-162.
func TestErlangScannerCParityReproducers(t *testing.T) {
	sc := ErlangExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-lookahead", "\r\n", 1, "101", "\x39\x8b\x80\xf8\x35\x25\x9a\x63\x44\x05\xe3\xda\xd6\x1f\xc2\x99\xf9\x30\x7e\x27\x50\x3b\x2c\xb7\x71\x4b\xf3\xb6\x36\xcc\x64\xb6\x1d\x2e\x37\x41\x19\xc8\xef\x8a\xdb\x21\xf1\x51\x6c\x7f\xe2\x38", false, 0, 0, 0, 3},
		{"mask-lookahead", "\n\n", 1, "100", "", false, 0, 0, 0, 3},
		{"ordinary-lookahead", "\n", 0, "111", "", false, 0, 0, 0, 2},
		{"range-lookahead", "\n\t\n\n", 1, "111", "", false, 0, 0, 0, 5},
		{"embedded-nul", "\"\"\"\n\x00\n\"\"\"", 0, "100", "", true, 145, 0, 9, 10},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}
