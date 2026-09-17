//go:build !grammar_subset || grammar_subset_fsharp

package grammarruntime

// RegisterFsharpSupport registers the scanner support for fsharp.
func RegisterFsharpSupport() {
	RegisterExternalScanner("fsharp", FsharpExternalScanner{})
}
