//go:build !grammar_subset || grammar_subset_odin

package grammarruntime

// RegisterOdinSupport registers the scanner support for odin.
func RegisterOdinSupport() {
	RegisterExternalScanner("odin", OdinExternalScanner{})
}
