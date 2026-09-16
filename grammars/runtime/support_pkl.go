//go:build !grammar_subset || grammar_subset_pkl

package grammarruntime

// RegisterPklSupport registers the scanner support for pkl.
func RegisterPklSupport() {
	RegisterExternalScanner("pkl", PklExternalScanner{})
}
