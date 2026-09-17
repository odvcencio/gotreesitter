//go:build !grammar_subset || grammar_subset_uxntal

package grammarruntime

// RegisterUxntalSupport registers the scanner support for uxntal.
func RegisterUxntalSupport() {
	RegisterExternalScanner("uxntal", UxntalExternalScanner{})
}
