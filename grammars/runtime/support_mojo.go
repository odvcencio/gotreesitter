//go:build !grammar_subset || grammar_subset_mojo

package grammarruntime

// RegisterMojoSupport registers the scanner support for mojo.
func RegisterMojoSupport() {
	RegisterExternalScanner("mojo", MojoExternalScanner{})
}
