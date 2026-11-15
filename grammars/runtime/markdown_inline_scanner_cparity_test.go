//go:build !grammar_subset

package grammarruntime

import (
	"strings"
	"testing"
)

// Reproductions of the pinned C scanner, scanner.c:81-92, 101-132, 181-182, 243-244, 306-307.
func TestMarkdownInlineScannerCParityReproducers(t *testing.T) {
	sc := MarkdownInlineExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-gopanic", "\x20", 1, "000000000000010", "\x00\x00", false, 0, 0, 0, 2},
		{"ordinary-lookahead", "\x72\x60\x00", 1, "001011000011101", "\x00\x01\x00\x00", true, 66, 1, 2, 4},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// scanner.c:113-126 uses size_t for the closing run, even when the opening run is uint8_t.
func TestMarkdownInlineScannerCParityLongClosingRun(t *testing.T) {
	src := "`x" + strings.Repeat("`", 257) + "x"
	checkScannerGroup1(t, MarkdownInlineExternalScanner{}, scannerGroup1Case{"long-closing-run", src, 0, "001000000000001", "", true, 66, 0, 1, uint32(len(src) + 1)})
}

// scanner.c:217-269 accepts a single tilde when the opening token is valid.
func TestMarkdownInlineScannerCParitySingleTilde(t *testing.T) {
	checkScannerGroup1(t, MarkdownInlineExternalScanner{}, scannerGroup1Case{"single-tilde", "~a", 0, "000000000010000", "", true, 62, 0, 1, 2})
}
