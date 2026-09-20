//go:build !grammar_subset || grammar_subset_elm

package grammarruntime

// RegisterElmSupport registers the scanner support for elm.
// elm_external_lex_states_gen.go registers the lex states in its own init,
// which is the shape cmd/ts2go emits for every other grammar.
func RegisterElmSupport() {
	RegisterExternalScanner("elm", ElmExternalScanner{})
}
