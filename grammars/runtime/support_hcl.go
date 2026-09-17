//go:build !grammar_subset || grammar_subset_hcl

package grammarruntime

// RegisterHclSupport registers the scanner support for hcl.
func RegisterHclSupport() {
	RegisterExternalScanner("hcl", HclExternalScanner{})
}
