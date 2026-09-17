//go:build !grammar_subset || grammar_subset_djot

package grammarruntime

// RegisterDjotSupport registers the scanner support for djot.
func RegisterDjotSupport() {
	RegisterExternalScanner("djot", DjotExternalScanner{})
}
