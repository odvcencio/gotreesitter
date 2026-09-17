//go:build !grammar_subset || grammar_subset_yuck

package grammarruntime

// RegisterYuckSupport registers the scanner support for yuck.
func RegisterYuckSupport() {
	RegisterExternalScanner("yuck", YuckExternalScanner{})
}
