//go:build !grammar_subset || grammar_subset_python

package grammarruntime

// RegisterPythonSupport registers the scanner support for python.
// python_external_lex_states_gen.go registers the lex states in its own
// init, which is the shape cmd/ts2go emits for every other grammar.
func RegisterPythonSupport() {
	RegisterExternalScanner("python", PythonExternalScanner{})
}
