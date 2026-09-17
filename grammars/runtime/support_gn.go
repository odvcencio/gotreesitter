//go:build !grammar_subset || grammar_subset_gn

package grammarruntime

// RegisterGnSupport registers the scanner support for gn.
func RegisterGnSupport() {
	RegisterExternalScanner("gn", GnExternalScanner{})
}
