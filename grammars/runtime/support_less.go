//go:build !grammar_subset || grammar_subset_less

package grammarruntime

// RegisterLessSupport registers the scanner support for less.
func RegisterLessSupport() {
	RegisterExternalScanner("less", LessExternalScanner{})
}
