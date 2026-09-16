//go:build !grammar_subset || grammar_subset_cobol

package grammarruntime

// RegisterCobolSupport registers the scanner support for cobol.
func RegisterCobolSupport() {
	RegisterExternalScanner("cobol", CobolExternalScanner{})
}
