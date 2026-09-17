//go:build !grammar_subset || grammar_subset_javascript

package grammarruntime

// RegisterJavascriptSupport registers the scanner support for javascript.
func RegisterJavascriptSupport() {
	RegisterExternalScanner("javascript", JavaScriptExternalScanner{})
}
