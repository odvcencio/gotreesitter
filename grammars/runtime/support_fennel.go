//go:build !grammar_subset || grammar_subset_fennel

package grammarruntime

// RegisterFennelSupport registers the scanner support for fennel.
func RegisterFennelSupport() {
	RegisterExternalScanner("fennel", FennelExternalScanner{})
}
