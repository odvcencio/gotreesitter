//go:build !grammar_subset || grammar_subset_teal

package grammarruntime

// RegisterTealSupport registers the scanner support for teal.
func RegisterTealSupport() {
	RegisterExternalScanner("teal", TealExternalScanner{})
}
