//go:build !grammar_subset || grammar_subset_pug

package grammarruntime

// RegisterPugSupport registers the scanner support for pug.
func RegisterPugSupport() {
	RegisterExternalScanner("pug", PugExternalScanner{})
}
