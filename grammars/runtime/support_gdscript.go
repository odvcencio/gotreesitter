//go:build !grammar_subset || grammar_subset_gdscript

package grammarruntime

// RegisterGdscriptSupport registers the scanner support for gdscript.
func RegisterGdscriptSupport() {
	RegisterExternalScanner("gdscript", GdscriptExternalScanner{})
}
