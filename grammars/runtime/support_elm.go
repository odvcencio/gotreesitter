//go:build !grammar_subset || grammar_subset_elm

package grammarruntime

// RegisterElmSupport registers the scanner support for elm.
func RegisterElmSupport() {
	RegisterExternalScanner("elm", ElmExternalScanner{})
	RegisterExternalLexStates("elm", elmExternalLexStates)
}
