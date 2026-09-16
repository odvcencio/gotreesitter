//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

// RegisterTsxSupport registers the scanner support for tsx.
func RegisterTsxSupport() {
	RegisterExternalScanner("tsx", TsxExternalScanner{})
}
