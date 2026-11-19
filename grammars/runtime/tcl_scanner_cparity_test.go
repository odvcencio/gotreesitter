//go:build !grammar_subset

package grammarruntime

import "testing"

// scanner.c:15-24 tests iswspace in the C locale.
func TestTclScannerCParityOrdinary(t *testing.T) {
	checkScannerGroup1(t, TclExternalScanner{}, scannerGroup1Case{"ordinary-found", "\x7d\xc2\xa0", 1, "10", "", true, 67, 1, 1, 2})
}
