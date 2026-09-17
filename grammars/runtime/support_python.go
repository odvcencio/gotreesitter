//go:build !grammar_subset || grammar_subset_python

package grammarruntime

// RegisterPythonSupport registers the scanner support for python.
func RegisterPythonSupport() {
	RegisterExternalScanner("python", PythonExternalScanner{})
	RegisterExternalLexStates("python", pythonExternalLexStates)
}
