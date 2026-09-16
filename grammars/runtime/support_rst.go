//go:build !grammar_subset || grammar_subset_rst

package grammarruntime

// RegisterRstSupport registers the scanner support for rst.
func RegisterRstSupport() {
	RegisterExternalScanner("rst", RstExternalScanner{})
}
