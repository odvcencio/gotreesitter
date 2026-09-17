//go:build !grammar_subset || grammar_subset_nim

package grammarruntime

// RegisterNimSupport registers the scanner support for nim.
func RegisterNimSupport() {
	RegisterExternalScanner("nim", NimExternalScanner{})
}
