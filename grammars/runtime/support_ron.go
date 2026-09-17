//go:build !grammar_subset || grammar_subset_ron

package grammarruntime

// RegisterRonSupport registers the scanner support for ron.
func RegisterRonSupport() {
	RegisterExternalScanner("ron", RonExternalScanner{})
}
