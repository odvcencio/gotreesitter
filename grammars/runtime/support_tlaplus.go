//go:build !grammar_subset || grammar_subset_tlaplus

package grammarruntime

// RegisterTlaplusSupport registers the scanner support for tlaplus.
func RegisterTlaplusSupport() {
	RegisterExternalScanner("tlaplus", TlaplusExternalScanner{})
}
