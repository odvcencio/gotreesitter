//go:build !grammar_subset || grammar_subset_wolfram

package grammarruntime

// RegisterWolframSupport registers the scanner support for wolfram.
func RegisterWolframSupport() {
	RegisterExternalScanner("wolfram", WolframExternalScanner{})
}
