//go:build !grammar_subset || grammar_subset_jsdoc

package grammarruntime

// RegisterJsdocSupport registers the scanner support for jsdoc.
func RegisterJsdocSupport() {
	RegisterExternalScanner("jsdoc", JsdocExternalScanner{})
}
