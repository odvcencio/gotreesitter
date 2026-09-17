//go:build !grammar_subset || grammar_subset_r

package grammarruntime

// RegisterRSupport registers the scanner support for r.
func RegisterRSupport() {
	RegisterExternalScanner("r", RExternalScanner{})
}
