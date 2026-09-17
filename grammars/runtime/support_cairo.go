//go:build !grammar_subset || grammar_subset_cairo

package grammarruntime

// RegisterCairoSupport registers the scanner support for cairo.
func RegisterCairoSupport() {
	RegisterExternalScanner("cairo", CairoExternalScanner{})
}
