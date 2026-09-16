//go:build !grammar_subset || grammar_subset_html

package grammarruntime

// RegisterHtmlSupport registers the scanner support for html.
func RegisterHtmlSupport() {
	RegisterExternalScanner("html", HTMLExternalScanner{})
}
