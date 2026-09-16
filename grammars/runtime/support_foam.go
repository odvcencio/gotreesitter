//go:build !grammar_subset || grammar_subset_foam

package grammarruntime

// RegisterFoamSupport registers the scanner support for foam.
func RegisterFoamSupport() {
	RegisterExternalScanner("foam", FoamExternalScanner{})
}
