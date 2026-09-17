//go:build !grammar_subset || grammar_subset_erlang

package grammarruntime

// RegisterErlangSupport registers the scanner support for erlang.
func RegisterErlangSupport() {
	RegisterExternalScanner("erlang", ErlangExternalScanner{})
}
