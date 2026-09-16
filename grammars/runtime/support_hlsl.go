//go:build !grammar_subset || grammar_subset_hlsl

package grammarruntime

// RegisterHlslSupport registers the scanner support for hlsl.
func RegisterHlslSupport() {
	RegisterExternalScanner("hlsl", HlslExternalScanner{})
}
