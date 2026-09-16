//go:build !grammar_subset || grammar_subset_dtd

package grammarruntime

// RegisterDtdSupport registers the scanner support for dtd.
func RegisterDtdSupport() {
	RegisterExternalScanner("dtd", DtdExternalScanner{})
}
