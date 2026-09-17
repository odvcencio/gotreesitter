//go:build !grammar_subset || grammar_subset_gleam

package grammarruntime

// RegisterGleamSupport registers the scanner support for gleam.
func RegisterGleamSupport() {
	RegisterExternalScanner("gleam", GleamExternalScanner{})
}
