//go:build !grammar_subset || grammar_subset_just

package grammarruntime

// RegisterJustSupport registers the scanner support for just.
func RegisterJustSupport() {
	RegisterExternalScanner("just", JustExternalScanner{})
}
