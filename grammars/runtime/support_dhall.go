//go:build !grammar_subset || grammar_subset_dhall

package grammarruntime

// RegisterDhallSupport registers the scanner support for dhall.
func RegisterDhallSupport() {
	RegisterExternalScanner("dhall", DhallExternalScanner{})
}
