//go:build !grammar_subset || grammar_subset_bitbake

package grammarruntime

// RegisterBitbakeSupport registers the scanner support for bitbake.
func RegisterBitbakeSupport() {
	RegisterExternalScanner("bitbake", BitbakeExternalScanner{})
}
