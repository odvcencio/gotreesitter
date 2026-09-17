//go:build !grammar_subset || grammar_subset_janet

package grammarruntime

// RegisterJanetSupport registers the scanner support for janet.
func RegisterJanetSupport() {
	RegisterExternalScanner("janet", JanetExternalScanner{})
}
