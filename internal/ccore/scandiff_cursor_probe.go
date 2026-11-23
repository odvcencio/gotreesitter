//go:build cgo && treesitter_c_parity

package ccore

// NewScannerCursorForDifferentialProbe exposes the same range cursor used by
// the parser to the out-of-package standalone scanner comparison.
func NewScannerCursorForDifferentialProbe(src []byte, r Range, at Length) ScannerCursor {
	return ScannerCursor{newIncludedCursor(src, []Range{r}, at)}
}
