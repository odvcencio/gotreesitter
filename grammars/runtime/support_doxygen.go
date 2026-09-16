//go:build !grammar_subset || grammar_subset_doxygen

package grammarruntime

// RegisterDoxygenSupport registers the scanner support for doxygen.
func RegisterDoxygenSupport() {
	RegisterExternalScanner("doxygen", DoxygenExternalScanner{})
}
