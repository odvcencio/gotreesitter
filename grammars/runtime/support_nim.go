//go:build (!grammar_subset || grammar_subset_nim) && !gotreesitter_no_copyleft

package grammarruntime

// RegisterNimSupport registers the scanner support for nim. See
// nim_no_copyleft_stub.go for the gotreesitter_no_copyleft build, which
// keeps this function's signature but drops its MPL-2.0-derived body.
func RegisterNimSupport() {
	RegisterExternalScanner("nim", NimExternalScanner{})
}
