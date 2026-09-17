//go:build !grammar_subset || grammar_subset_tablegen

package grammarruntime

// RegisterTablegenSupport registers the scanner support for tablegen.
func RegisterTablegenSupport() {
	RegisterExternalScanner("tablegen", TablegenExternalScanner{})
}
