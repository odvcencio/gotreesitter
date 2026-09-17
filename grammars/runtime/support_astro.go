//go:build !grammar_subset || grammar_subset_astro

package grammarruntime

// RegisterAstroSupport registers the scanner support for astro.
func RegisterAstroSupport() {
	RegisterExternalScanner("astro", AstroExternalScanner{})
}
