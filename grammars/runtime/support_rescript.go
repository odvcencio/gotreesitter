//go:build !grammar_subset || grammar_subset_rescript

package grammarruntime

// RegisterRescriptSupport registers the scanner support for rescript.
func RegisterRescriptSupport() {
	RegisterExternalScanner("rescript", RescriptExternalScanner{})
}
