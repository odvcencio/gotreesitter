//go:build !grammar_subset || grammar_subset_awk

package grammarruntime

// RegisterAwkSupport registers the scanner support for awk.
func RegisterAwkSupport() {
	RegisterExternalScanner("awk", AwkExternalScanner{})
}
