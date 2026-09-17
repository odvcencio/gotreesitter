//go:build !grammar_subset || grammar_subset_perl

package grammarruntime

// RegisterPerlSupport registers the scanner support for perl.
func RegisterPerlSupport() {
	RegisterExternalScanner("perl", PerlExternalScanner{})
}
