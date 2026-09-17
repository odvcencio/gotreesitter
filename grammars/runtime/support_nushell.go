//go:build !grammar_subset || grammar_subset_nushell

package grammarruntime

// RegisterNushellSupport registers the scanner support for nushell.
func RegisterNushellSupport() {
	RegisterExternalScanner("nushell", NushellExternalScanner{})
}
