//go:build !grammar_subset || grammar_subset_bicep

package grammarruntime

// RegisterBicepSupport registers the scanner support for bicep.
func RegisterBicepSupport() {
	RegisterExternalScanner("bicep", BicepExternalScanner{})
}
