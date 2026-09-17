//go:build !grammar_subset || grammar_subset_bash

package grammarruntime

// RegisterBashSupport registers the scanner support for bash.
func RegisterBashSupport() {
	RegisterExternalScanner("bash", BashExternalScanner{})
	RegisterExternalLexStates("bash", bashExternalLexStates)
}
