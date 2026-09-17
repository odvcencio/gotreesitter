//go:build !grammar_subset || grammar_subset_firrtl

package grammarruntime

// RegisterFirrtlSupport registers the scanner support for firrtl.
func RegisterFirrtlSupport() {
	RegisterExternalScanner("firrtl", FirrtlExternalScanner{})
}
