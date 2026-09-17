//go:build !grammar_subset || grammar_subset_beancount

package grammarruntime

// RegisterBeancountSupport registers the scanner support for beancount.
func RegisterBeancountSupport() {
	RegisterExternalScanner("beancount", BeancountExternalScanner{})
}
