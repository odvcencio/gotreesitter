//go:build !grammar_subset || grammar_subset_bash

package grammarruntime

// RegisterBashSupport registers the scanner support for bash.
// bash_external_lex_states_gen.go registers the lex states in its own init,
// which is the shape cmd/ts2go emits for every other grammar.
func RegisterBashSupport() {
	RegisterExternalScanner("bash", BashExternalScanner{})
}
