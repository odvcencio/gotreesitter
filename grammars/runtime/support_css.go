//go:build !grammar_subset || grammar_subset_css

package grammarruntime

// RegisterCssSupport registers the scanner support for css.
func RegisterCssSupport() {
	RegisterExternalScanner("css", CssExternalScanner{})
}
