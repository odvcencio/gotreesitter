//go:build !grammar_subset

package grammarruntime

import "testing"

// Pinned C scanner src/scanner.c:1331 distinguishes NUL from EOF.
func TestMarkdownScannerCParityReproducers(t *testing.T) {
	checkScannerGroup1(t, MarkdownExternalScanner{}, scannerGroup1Case{
		"ordinary-lookahead", "\x00", 0, "01001111111100111111111111110010111111010000010", "", false, 0, 0, 0, 2,
	})
}
