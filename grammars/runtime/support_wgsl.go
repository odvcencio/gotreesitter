//go:build !grammar_subset || grammar_subset_wgsl

package grammarruntime

// RegisterWgslSupport registers the scanner support for wgsl.
func RegisterWgslSupport() {
	RegisterExternalScanner("wgsl", WgslExternalScanner{})
}
