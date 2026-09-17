//go:build !grammar_subset || grammar_subset_typst

package grammarruntime

// RegisterTypstSupport registers the scanner support for typst.
func RegisterTypstSupport() {
	RegisterExternalScanner("typst", TypstExternalScanner{})
}
