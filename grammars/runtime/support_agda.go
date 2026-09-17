//go:build !grammar_subset || grammar_subset_agda

package grammarruntime

// RegisterAgdaSupport registers the scanner support for agda.
func RegisterAgdaSupport() {
	RegisterExternalScanner("agda", AgdaExternalScanner{})
}
