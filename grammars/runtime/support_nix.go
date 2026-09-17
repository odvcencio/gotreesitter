//go:build !grammar_subset || grammar_subset_nix

package grammarruntime

// RegisterNixSupport registers the scanner support for nix.
func RegisterNixSupport() {
	RegisterExternalScanner("nix", NixExternalScanner{})
}
