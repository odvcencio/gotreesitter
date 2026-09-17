//go:build !grammar_subset || grammar_subset_purescript

package grammarruntime

// RegisterPurescriptSupport registers the scanner support for purescript.
func RegisterPurescriptSupport() {
	RegisterExternalScanner("purescript", PurescriptExternalScanner{})
}
