//go:build !grammar_subset || grammar_subset_tcl

package grammarruntime

// RegisterTclSupport registers the scanner support for tcl.
func RegisterTclSupport() {
	RegisterExternalScanner("tcl", TclExternalScanner{})
}
