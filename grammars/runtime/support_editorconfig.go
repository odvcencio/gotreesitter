//go:build !grammar_subset || grammar_subset_editorconfig

package grammarruntime

// RegisterEditorconfigSupport registers the scanner support for editorconfig.
func RegisterEditorconfigSupport() {
	RegisterExternalScanner("editorconfig", EditorconfigExternalScanner{})
}
