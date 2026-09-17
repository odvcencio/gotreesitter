//go:build !grammar_subset || grammar_subset_templ

package grammarruntime

// RegisterTemplSupport registers the scanner support for templ.
func RegisterTemplSupport() {
	RegisterExternalScanner("templ", TemplExternalScanner{})
}
